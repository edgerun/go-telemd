// +build GPU_SUPPORT

package telemd

import (
	"errors"
	"fmt"
	"github.com/edgerun/telemd/internal/telem"
	"log"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

func x86Gpu() ([]string, error) {
	devices, err := execute("list_gpus")
	if err != nil {
		return []string{}, err
	}

	return devices, nil
}

func readTegraChipId() (string, error) {
	id, err := readFirstLine("/sys/module/tegra_fuse/parameters/tegra_chip_id")
	if err != nil {
		return "", err
	}
	return id, nil
}

// Returns the current frequency in Hz
// https://docs.nvidia.com/jetson/archives/l4t-archived/l4t-3231/index.html#page/Tegra%2520Linux%2520Driver%2520Package%2520Development%2520Guide%2Fpower_management_tx2_32.html%23wwpID0E0GD0HA
func readJetsonFrequency() (float64, error) {
	id, err := readTegraChipId()

	if err != nil {
		return -1, err
	}

	var folder string

	if id == "24" {
		// tx2
		folder = "17000000.gp10b"
	} else if id == "25" {
		// xavier nx
		folder = "17000000.gv11b"
	} else if id == "33" {
		// nano OR tx1
		folder = "57000000.gpu"
	} else {
		return -1, errors.New(fmt.Sprintf("unsupported tegra chip: %s", id))
	}

	line, err := readFirstLine(fmt.Sprintf("/sys/devices/gpu.0/devfreq/%s/cur_freq", folder))
	if err != nil {
		return -1, err
	}

	value, err := strconv.ParseFloat(line, 64)
	if err != nil {
		return -1, err
	}

	return value, nil
}

// Returns the current utilization of jetson gpu
func readJetsonGpuUtilization() (float64, error) {
	// value needs to be divided by 10, i.e. 999 => 99.9%
	line, err := readFirstLine("/sys/devices/gpu.0/load")
	if err != nil {
		return -1, err
	}

	value, err := strconv.ParseFloat(line, 64)
	if err != nil {
		return -1, err
	}

	return value / 10, nil
}

func arm64Gpu() ([]string, error) {
	// this only works on jetson devices!
	// other way to list all gpu devices, though without knowing the jetson device:
	// all gpu devices are mounted in /sys/devices, i.e. /sys/devices/gpu.0 <- works also in container (L4T base image)

	// https://forums.developer.nvidia.com/t/how-to-identify-nano/72160
	id, err := readTegraChipId()
	if err != nil {
		return nil, err
	}

	if id == "64" {
		return []string{"0-Jetson TK1"}, nil
	} else if id == "33" {
		// according to the blog post above, jetson tx1 has the same id as tx1
		// problem: /proc/device-tree/model not available in container
		return []string{"0-Jetson Nano"}, nil
	} else if id == "24" {
		return []string{"0-Jetson TX2"}, nil
	} else if id == "25" {
		return []string{"0-Jetson Xavier NX"}, nil
	} else {
		return []string{}, errors.New(fmt.Sprintf("unsupported tegra chip: %s", id))
	}
}

func gpuDevices() map[int]string {
	arch := runtime.GOARCH
	var gpus []string
	var err error
	if arch == "arm64" {
		gpus, err = arm64Gpu()
	} else if arch == "amd64" {
		gpus, err = x86Gpu()
	} else {
		return map[int]string{}
	}

	if err != nil {
		log.Fatalln("error fetching gpu devices: ", err.Error())
		return map[int]string{}
	}

	devices := map[int]string{}

	for _, gpu := range gpus {
		split := strings.Split(gpu, "-")
		id, _ := strconv.Atoi(split[0])

		devices[id] = split[1]
	}

	return devices
}

func (instr Arm64GpuFrequencyInstrument) MeasureAndReport(channel telem.TelemetryChannel) {
	frequencyInHz, err := readJetsonFrequency()
	if err != nil {
		log.Println("Error reading jetson gpu frequency: ", err)
		return
	}

	frequencyInMHz := frequencyInHz / (1_000_000)
	channel.Put(telem.NewTelemetry("gpu_freq"+telem.TopicSeparator+"0", frequencyInMHz))
}

func (instr X86GpuFrequencyInstrument) MeasureAndReport(channel telem.TelemetryChannel) {
	var wg sync.WaitGroup
	wg.Add(len(instr.Devices))
	defer wg.Wait()

	measureAndReport := func(id int) {
		defer wg.Done()

		// gpu_freq already returns MHz
		frequencies, err := execute("gpu_freq", strconv.Itoa(id))
		if err != nil {
			log.Println("Error reading gpu measurements", err)
		}

		if len(frequencies) != 1 {
			log.Println("Expected 1 cpu freqency measurement but were ", len(frequencies))
			return
		}

		//Format: id-name-measure-value
		values := strings.Split(frequencies[0], "-")
		frequency, err := strconv.ParseFloat(values[3], 64)
		if err != nil {
			log.Println("Expected number from gpu frequency, but got: ", values[3])
			return
		}

		channel.Put(telem.NewTelemetry("gpu_freq"+telem.TopicSeparator+strconv.Itoa(id), frequency))
	}

	for id, _ := range instr.Devices {
		go measureAndReport(id)
	}
}

func (instr DefaultGpuFrequencyInstrument) MeasureAndReport(channel telem.TelemetryChannel) {
	// per default no gpu support
}

func (instr DefaultGpuUtilInstrument) MeasureAndReport(channel telem.TelemetryChannel) {
	// per default no gpu support
}

func (instr Arm64GpuUtilInstrument) MeasureAndReport(channel telem.TelemetryChannel) {
	gpuUtil, err := readJetsonGpuUtilization()
	if err != nil {
		log.Println("Error reading jetson gpu frequency: ", err)
		return
	}

	channel.Put(telem.NewTelemetry("gpu_util"+telem.TopicSeparator+"0", gpuUtil))
}

func (instr X86GpuUtilInstrument) MeasureAndReport(channel telem.TelemetryChannel) {
	var wg sync.WaitGroup
	wg.Add(len(instr.Devices))
	defer wg.Wait()

	measureAndReport := func(id int) {
		defer wg.Done()

		// gpu_util already returns percentage
		frequencies, err := execute("gpu_util", strconv.Itoa(id))
		if err != nil {
			log.Println("Error reading gpu utilization", err)
		}

		if len(frequencies) != 1 {
			log.Println("Expected 1 gpu utilization measurement but were ", len(frequencies))
			return
		}

		//Format: id-name-measure-value
		values := strings.Split(frequencies[0], "-")
		frequency, err := strconv.ParseFloat(values[3], 64)
		if err != nil {
			log.Println("Expected number from gpu_util, but got: ", values[3])
			return
		}

		channel.Put(telem.NewTelemetry("gpu_util"+telem.TopicSeparator+strconv.Itoa(id), frequency))
	}

	for id, _ := range instr.Devices {
		go measureAndReport(id)
	}
}

type DefaultGpuFrequencyInstrument struct {
}

type Arm64GpuFrequencyInstrument struct {
	Devices map[int]string
}

type X86GpuFrequencyInstrument struct {
	Devices map[int]string
}

type DefaultGpuUtilInstrument struct {
}

type Arm64GpuUtilInstrument struct {
	Devices map[int]string
}

type X86GpuUtilInstrument struct {
	Devices map[int]string
}

func (d defaultInstrumentFactory) NewGpuFrequencyInstrument(devices map[int]string) Instrument {
	return DefaultGpuFrequencyInstrument{}
}

func (a arm64InstrumentFactory) NewGpuFrequencyInstrument(devices map[int]string) Instrument {
	return Arm64GpuFrequencyInstrument{devices}
}

func (x x86InstrumentFactory) NewGpuFrequencyInstrument(devices map[int]string) Instrument {
	return X86GpuFrequencyInstrument{devices}
}

func (d defaultInstrumentFactory) NewGpuUtilInstrument(devices map[int]string) Instrument {
	return DefaultGpuUtilInstrument{}
}

func (a arm64InstrumentFactory) NewGpuUtilInstrument(devices map[int]string) Instrument {
	return Arm64GpuUtilInstrument{devices}
}

func (x x86InstrumentFactory) NewGpuUtilInstrument(devices map[int]string) Instrument {
	return X86GpuUtilInstrument{devices}
}
