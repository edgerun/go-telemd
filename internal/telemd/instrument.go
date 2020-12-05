package telemd

import (
	"bufio"
	"github.com/edgerun/telemd/internal/telem"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Instrument interface {
	// MeasureAndReport executes a measurement, creates appropriate Telemetry
	// instances, and put them into the given TelemetryChannel.
	MeasureAndReport(telemetry telem.TelemetryChannel)
}

type InstrumentFactory interface {
	NewCpuFrequencyInstrument() Instrument
	NewCpuUtilInstrument() Instrument
	NewLoadInstrument() Instrument
	NewProcsInstrument() Instrument
	NewRamInstrument() Instrument
	NewNetworkDataRateInstrument([]string) Instrument
	NewDiskDataRateInstrument([]string) Instrument
	NewGpuFrequencyInstrument(map[int]string) Instrument
	NewGpuUtilInstrument(map[int]string) Instrument
	NewCgroupCpuInstrument() Instrument
	NewCgroupBlkioInstrument() Instrument
	NewCgroupNetworkInstrument() Instrument
}

type CpuInfoFrequencyInstrument struct{}
type CpuScalingFrequencyInstrument struct{}
type CpuUtilInstrument struct{}
type LoadInstrument struct{}
type ProcsInstrument struct{}
type RamInstrument struct{}
type NetworkDataRateInstrument struct {
	Devices []string
}
type DiskDataRateInstrument struct {
	Devices []string
}
type CgroupCpuInstrument struct{}
type CgroupBlkioInstrument struct{}
type CgroupNetworkInstrument struct {
	pids map[string]string
}

var DisabledInstrument = noopInstrument{}

type noopInstrument struct{}

func (noopInstrument) MeasureAndReport(telem.TelemetryChannel) {
	// noop
}

func (CpuUtilInstrument) MeasureAndReport(channel telem.TelemetryChannel) {
	then := readCpuUtil()
	time.Sleep(500 * time.Millisecond)
	now := readCpuUtil()

	val := (now[0] - then[0] + now[2] - then[2]) * 100. / (now[0] - then[0] + now[2] - then[2] + now[3] - then[3])
	channel.Put(telem.NewTelemetry("cpu", val))
}

func (CpuInfoFrequencyInstrument) MeasureAndReport(channel telem.TelemetryChannel) {
	file, err := os.Open("/proc/cpuinfo")
	check(err)
	defer func() {
		err = file.Close()
		if err != nil {
			log.Fatal(err)
		}
	}()

	scanner := bufio.NewScanner(file)

	var sum float64

	for scanner.Scan() {
		text := scanner.Text()

		if strings.HasPrefix(text, "cpu MHz") {
			split := strings.Split(text, ":")
			strval := strings.Trim(split[1], " ")
			if val, err := strconv.ParseFloat(strval, 64); err == nil {
				sum += val
			} else {
				log.Println("could not parse value: '", strval, "' to float:", err)
			}
		}

		if err := scanner.Err(); err != nil {
			log.Fatal(err)
		}
	}

	channel.Put(telem.NewTelemetry("freq", sum))
}

var cpuScalingFiles, _ = filepath.Glob("/sys/devices/system/cpu/cpu[0-9]*/cpufreq/scaling_cur_freq")

func (c CpuScalingFrequencyInstrument) MeasureAndReport(channel telem.TelemetryChannel) {
	var sum int64

	for _, match := range cpuScalingFiles {
		value, err := readLineAndParseInt(match)
		check(err)
		sum += value
	}

	channel.Put(telem.NewTelemetry("freq", float64(sum)))
}

func (LoadInstrument) MeasureAndReport(channel telem.TelemetryChannel) {
	text, err := readFirstLine("/proc/loadavg")
	check(err)

	parts := strings.Split(text, " ")

	l1 := parts[0]
	l5 := parts[1]
	//l15 := parts[2]

	if val, err := strconv.ParseFloat(l1, 64); err == nil {
		channel.Put(telem.NewTelemetry("load1", val))
	}

	if val, err := strconv.ParseFloat(l5, 64); err == nil {
		channel.Put(telem.NewTelemetry("load5", val))
	}

	//if val, err := strconv.ParseFloat(l15, 64); err == nil {
	//	channel.Put(NewTelemetry("load15", val))
	//}

}

func (ProcsInstrument) MeasureAndReport(channel telem.TelemetryChannel) {
	text, err := readFirstLine("/proc/loadavg")
	check(err)

	fields := strings.Split(text, " ")

	procs := strings.Split(fields[3], "/")[0]

	if val, err := strconv.ParseFloat(procs, 64); err == nil {
		channel.Put(telem.NewTelemetry("procs", val))
	}
}

func (instr *NetworkDataRateInstrument) MeasureAndReport(channel telem.TelemetryChannel) {
	var wg sync.WaitGroup
	wg.Add(len(instr.Devices))
	defer wg.Wait()

	measureAndReport := func(device string) {
		rxPath := "/sys/class/net/" + device + "/statistics/rx_bytes"
		txPath := "/sys/class/net/" + device + "/statistics/tx_bytes"

		rxThen, err := readLineAndParseInt(rxPath)
		if err != nil {
			log.Println("error while reading path", rxPath, err)
			return
		}
		txThen, err := readLineAndParseInt(txPath)
		if err != nil {
			log.Println("error while reading path", txPath, err)
			return
		}
		time.Sleep(1 * time.Second)

		rxNow, err := readLineAndParseInt(rxPath)
		if err != nil {
			log.Println("error while reading path", rxPath, err)
			return
		}
		txNow, err := readLineAndParseInt(txPath)
		if err != nil {
			log.Println("error while reading path", txPath, err)
			return
		}

		channel.Put(telem.NewTelemetry("tx"+telem.TopicSeparator+device, float64((txNow-txThen)/1000)))
		channel.Put(telem.NewTelemetry("rx"+telem.TopicSeparator+device, float64((rxNow-rxThen)/1000)))
		wg.Done()
	}

	for _, device := range instr.Devices {
		go measureAndReport(device)
	}
}

const sectorSize = 512

func (instr *DiskDataRateInstrument) MeasureAndReport(channel telem.TelemetryChannel) {
	var wg sync.WaitGroup
	wg.Add(len(instr.Devices))
	defer wg.Wait()

	measureAndReport := func(device string) {
		defer wg.Done()

		statsThen, err := readBlockDeviceStats(device)
		if err != nil {
			log.Println("error reading block device stats", device, err)
			return
		}

		time.Sleep(1 * time.Second)
		statsNow, err := readBlockDeviceStats(device)
		if err != nil {
			log.Println("error reading block device stats", device, err)
			return
		}

		rd := (statsNow[2] - statsThen[2]) * sectorSize
		wr := (statsNow[6] - statsThen[6]) * sectorSize

		channel.Put(telem.NewTelemetry("rd"+telem.TopicSeparator+device, float64(rd)/1000))
		channel.Put(telem.NewTelemetry("wr"+telem.TopicSeparator+device, float64(wr)/1000))
	}

	for _, device := range instr.Devices {
		go measureAndReport(device)
	}
}

func (instr RamInstrument) MeasureAndReport(channel telem.TelemetryChannel) {
	meminfo := readMeminfo()

	totalString, ok := meminfo["MemTotal"]
	if !ok {
		return
	}
	total, err := parseMeminfoString(totalString)
	if err != nil {
		log.Println("Error parsing MemTotal string", totalString, err)
	}

	freeString, ok := meminfo["MemAvailable"]
	if !ok {
		return
	}
	free, err := parseMeminfoString(freeString)
	if err != nil {
		log.Println("Error parsing MemFree string", freeString, err)
	}

	channel.Put(telem.NewTelemetry("ram", float64(total-free)))
}

func (CgroupCpuInstrument) MeasureAndReport(channel telem.TelemetryChannel) {
	dirs := listFilterDir("/sys/fs/cgroup/cpuacct/docker", func(info os.FileInfo) bool {
		return info.IsDir() && info.Name() != "." && info.Name() != ".."
	})

	for _, containerId := range dirs {
		dataFile := "/sys/fs/cgroup/cpuacct/docker/" + containerId + "/cpuacct.usage"
		value, err := readLineAndParseInt(dataFile)
		if err != nil {
			log.Println("error reading data file", dataFile, err)
			continue
		}
		channel.Put(telem.NewTelemetry("cgrp_cpu/"+containerId[:12], float64(value)))
	}
}

func (c *CgroupNetworkInstrument) MeasureAndReport(channel telem.TelemetryChannel) {
	containerIds := listFilterDir("/sys/fs/cgroup/cpuacct/docker", func(info os.FileInfo) bool {
		return info.IsDir() && info.Name() != "." && info.Name() != ".."
	})

	if len(containerIds) == 0 {
		return
	}

	for _, containerId := range containerIds {
		pid, ok := c.pids[containerId]

		if !ok {
			// refresh pids
			pids, err := containerProcessIds()
			if err != nil {
				log.Println("unable to get container process ids", err)
				continue
			}
			c.pids = pids

			pid, ok = c.pids[containerId]
			if !ok {
				log.Println("could not get pid of container after refresh", containerId)
				continue
			}
		}

		rx, tx, err := readTotalProcessNetworkStats(pid)
		if err != nil {
			if os.IsNotExist(err) {
				delete(c.pids, containerId) // delete now and wait for next iteration to refresh
			} else {
				log.Println("error parsing network stats of pid", pid, err, err)
			}
			continue
		}

		channel.Put(telem.NewTelemetry("cgrp_net/"+containerId[:12], float64(rx+tx)))
	}
}

func readBlkioTotal(path string) (val int64, err error) {
	visitorErr := visitLines(path, func(line string) bool {
		if strings.HasPrefix(line, "Total") {
			val, err = strconv.ParseInt(strings.Split(line, " ")[1], 10, 64)
			return false
		}
		return true
	})

	if visitorErr != nil {
		return val, visitorErr
	}

	return
}

func (CgroupBlkioInstrument) MeasureAndReport(channel telem.TelemetryChannel) {
	dirs := listFilterDir("/sys/fs/cgroup/blkio/docker", func(info os.FileInfo) bool {
		return info.IsDir() && info.Name() != "." && info.Name() != ".."
	})

	for _, containerId := range dirs {
		dataFile := "/sys/fs/cgroup/blkio/docker/" + containerId + "/blkio.throttle.io_service_bytes"
		value, err := readBlkioTotal(dataFile)
		if err != nil {
			log.Println("error reading data file", dataFile, err)
			continue
		}
		channel.Put(telem.NewTelemetry("cgrp_blkio/"+containerId[:12], float64(value)))
	}
}

type defaultInstrumentFactory struct{}

type arm32InstrumentFactory struct {
	defaultInstrumentFactory
}

type arm64InstrumentFactory struct {
	defaultInstrumentFactory
}

type x86InstrumentFactory struct {
	defaultInstrumentFactory
}

func (d defaultInstrumentFactory) NewCpuFrequencyInstrument() Instrument {
	return CpuScalingFrequencyInstrument{}
}

func (d defaultInstrumentFactory) NewCpuUtilInstrument() Instrument {
	return CpuUtilInstrument{}
}

func (d defaultInstrumentFactory) NewLoadInstrument() Instrument {
	return LoadInstrument{}
}

func (d defaultInstrumentFactory) NewProcsInstrument() Instrument {
	return ProcsInstrument{}
}

func (d defaultInstrumentFactory) NewRamInstrument() Instrument {
	return RamInstrument{}
}

func (d defaultInstrumentFactory) NewNetworkDataRateInstrument(devices []string) Instrument {
	return &NetworkDataRateInstrument{devices}
}

func (d defaultInstrumentFactory) NewDiskDataRateInstrument(devices []string) Instrument {
	return &DiskDataRateInstrument{devices}
}

func (d defaultInstrumentFactory) NewCgroupCpuInstrument() Instrument {
	return CgroupCpuInstrument{}
}

func (d defaultInstrumentFactory) NewCgroupBlkioInstrument() Instrument {
	return CgroupBlkioInstrument{}
}

func (d defaultInstrumentFactory) NewCgroupNetworkInstrument() Instrument {
	pidMap, err := containerProcessIds()

	if err != nil {
		log.Println("unable to get process ids of containers", err)
	}

	return &CgroupNetworkInstrument{
		pids: pidMap,
	}
}

func NewInstrumentFactory(arch string) InstrumentFactory {
	switch arch {
	case "amd64":
		return x86InstrumentFactory{}
	case "arm":
		return arm32InstrumentFactory{}
	case "arm64":
		return arm64InstrumentFactory{}
	default:
		log.Printf("Unknown arch %s, returning default factory", arch)
	}

	return defaultInstrumentFactory{}
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
