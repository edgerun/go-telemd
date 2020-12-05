package telemd

import (
	"github.com/edgerun/telemd/internal/telem"
	"log"
	"runtime"
	"sync"
	"time"
)

type Daemon struct {
	cfg               *Config
	cmds              *commandChannel
	isPausedByCommand bool
	telemetry         telem.TelemetryChannel
	instruments       map[string]Instrument

	tickers map[string]TelemetryTicker
}

func NewDaemon(cfg *Config) *Daemon {
	td := &Daemon{
		cfg:       cfg,
		telemetry: telem.NewTelemetryChannel(),
		cmds:      newCommandChannel(),
		tickers:   make(map[string]TelemetryTicker),
	}

	td.initInstruments(NewInstrumentFactory(runtime.GOARCH))
	td.initTickers()

	return td
}

func (daemon *Daemon) initInstruments(factory InstrumentFactory) {
	cfg := daemon.cfg

	instruments := map[string]Instrument{
		"cpu":        factory.NewCpuUtilInstrument(),
		"freq":       factory.NewCpuFrequencyInstrument(),
		"load":       factory.NewLoadInstrument(),
		"ram":        factory.NewRamInstrument(),
		"procs":      factory.NewProcsInstrument(),
		"net":        factory.NewNetworkDataRateInstrument(cfg.Instruments.Net.Devices),
		"disk":       factory.NewDiskDataRateInstrument(cfg.Instruments.Disk.Devices),
		"gpu_freq":   factory.NewGpuFrequencyInstrument(cfg.Instruments.Gpu.Devices),
		"gpu_util":   factory.NewGpuUtilInstrument(cfg.Instruments.Gpu.Devices),
		"cgrp_cpu":   factory.NewCgroupCpuInstrument(),
		"cgrp_blkio": factory.NewCgroupBlkioInstrument(),
		"cgrp_net":   factory.NewCgroupNetworkInstrument(),
	}

	// remove instruments that were disabled by the factory
	for name, instr := range instruments {
		if instr == DisabledInstrument {
			delete(instruments, name)
		}
	}

	if cfg.Instruments.Disable != nil && (len(cfg.Instruments.Disable) > 0) {
		log.Println("disabling instruments", cfg.Instruments.Disable)
		for _, instr := range cfg.Instruments.Disable {
			delete(instruments, instr)
		}
		daemon.instruments = instruments
	} else if cfg.Instruments.Enable != nil && (len(cfg.Instruments.Enable) > 0) {
		log.Println("enabling instruments", cfg.Instruments.Enable)
		daemon.instruments = make(map[string]Instrument, len(cfg.Instruments.Enable))

		for _, key := range cfg.Instruments.Enable {
			if value, ok := instruments[key]; ok {
				daemon.instruments[key] = value
			}
		}
	} else {
		daemon.instruments = instruments
	}
}

func (daemon *Daemon) initTickers() {
	for k, instrument := range daemon.instruments {
		period, ok := daemon.cfg.Instruments.Periods[k]
		if !ok {
			log.Println("warning: no period assigned for instrument", k, "using 1")
			period = 1 * time.Second
		}
		ticker := NewTelemetryTicker(instrument, daemon.telemetry, period)
		daemon.tickers[k] = ticker
	}
}

func (daemon *Daemon) startTickers() *sync.WaitGroup {
	var wg sync.WaitGroup

	// start tickers and add to wait group
	for _, ticker := range daemon.tickers {
		go func(t TelemetryTicker) {
			wg.Add(1)
			t.Run()
			wg.Done()
		}(ticker)
	}

	return &wg
}

func (daemon *Daemon) Run() {
	var wg sync.WaitGroup
	wg.Add(2)

	// run command loop
	go func() {
		daemon.runCommandLoop()
		wg.Done()
	}()

	// run tickers
	go func() {
		daemon.startTickers().Wait()
		wg.Done()
	}()

	wg.Wait()
	time.Sleep(1 * time.Second) // TODO: properly wait for all tickers to exit
	log.Println("closing telemetry channel")
	daemon.telemetry.Close()
}

func (daemon *Daemon) Send(command Command) {
	daemon.cmds.channel <- command
}

func (daemon *Daemon) Stop() {
	// stop accepting Daemon channel
	daemon.cmds.stop <- true

	// stop tickers
	for k, ticker := range daemon.tickers {
		log.Println("stopping ticker " + k)
		ticker.Stop()
	}
}
