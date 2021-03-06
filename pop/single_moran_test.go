package pop

import (
	"github.com/mingzhi/gomath/random"
	"github.com/mingzhi/gomath/stat/desc"
	"math"
	"math/rand"
	"runtime"
	"testing"
	"time"
)

func runOnePop(popSize, genomeLen int, mutRate, traRate float64, frag, numGen int) *Pop {
	var p *Pop
	var alphabet []byte

	p = New()
	alphabet = []byte{1, 2, 3, 4}

	// randome number source.
	src := random.NewLockedSource(rand.NewSource(time.Now().UnixNano()))
	r := rand.New(src)

	NewRandomPopGenerator(r, popSize, genomeLen, alphabet).Operate(p)

	moranEvent := &Event{
		Ops: NewMoranSampler(r),
		Pop: p,
	}

	mutationEvent := &Event{
		Rate: mutRate,
		Ops:  NewSimpleMutator(r, alphabet),
		Pop:  p,
	}

	constantFrag := NewConstantFrag(frag)
	transferEvent := &Event{
		Rate: traRate,
		Ops:  NewSimpleTransfer(constantFrag, r),
		Pop:  p,
	}

	poisson := random.NewPoisson(float64(genomeLen)*(mutRate+traRate), src)

	eventChan := make(chan *Event)

	go func() {
		defer close(eventChan)
		for k := 0; k < numGen; k++ {
			eventChan <- moranEvent
			count := poisson.Int()
			for c := 0; c < count; c++ {
				eventChan <- Emit([]*Event{
					mutationEvent,
					transferEvent,
				}, r)
			}
		}
	}()

	Evolve(eventChan)

	return p
}

func TestSingleMoran(t *testing.T) {
	// set number of CPUs for using
	runtime.GOMAXPROCS(runtime.NumCPU())
	// population parameters
	popSizeArr := []int{100}
	mutRates := []float64{0.01, 0.001}
	traRates := []float64{0, 0.001, 0.01}
	genomeLen := 100
	frag := 10
	replicates := 10

	for i, popSize := range popSizeArr {
		mutRate := mutRates[i]
		numGen := 10 * popSize * popSize
		for _, traRate := range traRates {
			mean := desc.NewMean()
			vard := desc.NewVarianceWithBiasCorrection()
			for j := 0; j < replicates; j++ {
				p := runOnePop(popSize, genomeLen, mutRate, traRate, frag, numGen)
				d, _ := CalcKs(10, p)
				mean.Increment(d)
				vard.Increment(d)
			}

			res := mean.GetResult()
			ste := math.Sqrt(vard.GetResult() / float64(vard.GetN()))
			nu := float64(popSize) * mutRate
			gamma := float64(frag) * traRate
			exp := nu / (1 + gamma + 4.0/3.0*nu)
			if math.IsNaN(res) {
				t.Error("Nan for result")
			}
			if math.Abs(res-exp) > 3.0*ste {
				t.Errorf("n = %d, u = %f, t = %f, Expected %f, but got %f, at standard error %f\n", popSize, mutRate, traRate, exp, res, ste)
			}
		}
	}
}

func BenchmarkSingleMoran(b *testing.B) {
	popSize := 100
	mutRate := 0.001
	traRate := 0.001
	genomeLen := 100
	frag := 10
	runOnePop(popSize, genomeLen, mutRate, traRate, frag, b.N)
}
