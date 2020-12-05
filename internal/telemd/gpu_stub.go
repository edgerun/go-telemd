// +build !GPU_SUPPORT

package telemd

func gpuDevices() map[int]string {
	return map[int]string{}
}

func (d defaultInstrumentFactory) NewGpuFrequencyInstrument(map[int]string) Instrument {
	return DisabledInstrument
}

func (a arm64InstrumentFactory) NewGpuFrequencyInstrument(map[int]string) Instrument {
	return DisabledInstrument
}

func (x x86InstrumentFactory) NewGpuFrequencyInstrument(map[int]string) Instrument {
	return DisabledInstrument
}

func (d defaultInstrumentFactory) NewGpuUtilInstrument(map[int]string) Instrument {
	return DisabledInstrument
}

func (a arm64InstrumentFactory) NewGpuUtilInstrument(map[int]string) Instrument {
	return DisabledInstrument
}

func (x x86InstrumentFactory) NewGpuUtilInstrument(map[int]string) Instrument {
	return DisabledInstrument
}
