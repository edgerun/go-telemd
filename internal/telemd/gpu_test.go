// +build GPU_SUPPORT

package telemd

import (
	"github.com/edgerun/telemd/internal/telem"
	"log"
	"testing"
	"time"
)

func TestX86GpuFrequencyInstrument_MeasureAndReport(t *testing.T) {
	instrument := X86GpuFrequencyInstrument{map[int]string{0: "dummy_gpu"}}
	tc := telem.NewTelemetryChannel()

	go instrument.MeasureAndReport(tc)

	select {
	case <-time.After(1 * time.Second):
		t.Error("Timeout waiting for GPU frequency")
	case t1 := <-tc.Channel():
		if t1.Value < 0 {
			t.Error("Unexpected negative GPU frequency")
		}
		log.Printf("%.4f\n", t1.Value)
	}

}

func TestX86GpuUtilInstrument_MeasureAndReport(t *testing.T) {
	instrument := X86GpuUtilInstrument{map[int]string{0: "dummy_gpu"}}
	tc := telem.NewTelemetryChannel()

	go instrument.MeasureAndReport(tc)

	select {
	case <-time.After(1 * time.Second):
		t.Error("Timeout waiting for GPU utilization")
	case t1 := <-tc.Channel():
		if t1.Value < 0 {
			t.Error("Unexpected negative GPU utilization")
		}
		log.Printf("%.4f\n", t1.Value)
	}

}
