package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/mingzhi/popsimu/pop"
	"github.com/mingzhi/popsimu/simu"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
)

var (
	workspace string // workspace
	config    string // config file
	prefix    string // prefix
	outdir    string // output folder
	numRep    int    // number of replicates.
	genStep   int    // number of generations for each step
	genTime   int    // number of times

	outfile *os.File
	encoder *json.Encoder
)

func init() {
	// flags
	flag.StringVar(&workspace, "w", "", "workspace")
	flag.StringVar(&config, "c", "config.yaml", "parameter set file")
	flag.StringVar(&prefix, "p", "test", "prefix")
	flag.StringVar(&outdir, "o", "", "output directory")
	flag.IntVar(&numRep, "n", 10, "number of replicates")
	flag.IntVar(&genStep, "s", 1000, "number of generations for each step")
	flag.IntVar(&genTime, "t", 1, "number of times")

	flag.Parse()

	outFileName := prefix + "_res.json"
	outFilePath := filepath.Join(workspace, outdir, outFileName)
	outfile, err := os.Create(outFilePath)
	if err != nil {
		panic(err)
	}
	defer outfile.Close()

	encoder = json.NewEncoder(outfile)
}

func main() {
	ncpu := runtime.NumCPU()
	runtime.GOMAXPROCS(ncpu)
	// parse parameter sets.
	filePath := filepath.Join(workspace, config)
	fmt.Printf("config file path: %s\n", filePath)
	parSets := parseParameterSets(filePath)
	for i := 0; i < len(parSets); i++ {
		fmt.Println(parSets[i])
	}
	popConfigCombinations := generatePopConfigs(parSets)

	jobChan := make(chan []pop.Config)
	go func() {
		defer close(jobChan)
		for _, comb := range popConfigCombinations {
			jobChan <- comb
		}
	}()

	resultChan := make(chan Results)
	done := make(chan bool)
	for i := 0; i < ncpu; i++ {
		go func() {
			for comb := range jobChan {
				res := Results{PopConfigs: comb}
				for j := 0; j < numRep; j++ {
					pops := createPops(comb)
					numGen := 0
					for i := 0; i < genTime; i++ {
						simu.RunMoran(pops, comb, genStep)
						numGen += genStep
						calcResults := calculateResults(pops, numGen)
						res.CalcResults = append(res.CalcResults, calcResults...)
					}
				}
				resultChan <- res
			}
			done <- true
		}()
	}

	go func() {
		defer close(resultChan)
		for i := 0; i < ncpu; i++ {
			<-done
		}
	}()

	for res := range resultChan {
		encoder.Encode(res)
		if err := outfile.Sync(); err != nil {
			panic(err)
		}
	}
}

type ParameterSet struct {
	Sizes            []int
	Lengths          []int
	MutationRates    []float64
	TransferInRates  []float64
	TransferInFrags  []int
	TransferOutRates []float64
	TransferOutFrags []int
	Alphabet         []byte
}

func (p ParameterSet) String() string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "Sizes: %v\n", p.Sizes)
	fmt.Fprintf(&b, "Lengths: %v\n", p.Lengths)
	fmt.Fprintf(&b, "Mutation rates: %v\n", p.MutationRates)
	fmt.Fprintf(&b, "Transfer in rates: %v\n", p.TransferInRates)
	fmt.Fprintf(&b, "Transfer in frags: %v\n", p.TransferInFrags)
	fmt.Fprintf(&b, "Transfer out rates: %v\n", p.TransferOutRates)
	fmt.Fprintf(&b, "Transfer out frags: %v\n", p.TransferOutFrags)

	return b.String()
}

func parseParameterSets(filePath string) []ParameterSet {
	f, err := os.Open(filePath)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	data, err := ioutil.ReadAll(f)

	sets := []ParameterSet{}
	if err := yaml.Unmarshal(data, &sets); err != nil {
		panic(err)
	}
	return sets
}

type Results struct {
	PopConfigs  []pop.Config
	CalcResults []CalcRes
}

type CalcRes struct {
	Index  []int
	Ks, Vd float64
	NumGen int
}

func calculateResults(pops []*pop.Pop, numGen int) []CalcRes {
	calcResults := []CalcRes{}
	for i := 0; i < len(pops); i++ {
		p1 := pops[i]
		ks, vd := pop.CalcKs(p1)
		res := CalcRes{
			Index:  []int{i},
			Ks:     ks,
			Vd:     vd,
			NumGen: numGen,
		}

		calcResults = append(calcResults, res)
		for j := i + 1; j < len(pops); j++ {
			p2 := pops[j]
			ks, vd := pop.CrossKs(p1, p2)
			res := CalcRes{
				Index:  []int{i, j},
				Ks:     ks,
				Vd:     vd,
				NumGen: numGen,
			}
			calcResults = append(calcResults, res)
		}
	}

	return calcResults
}

func generatePopConfigs(parSets []ParameterSet) [][]pop.Config {
	popCfgs := [][]pop.Config{}
	for i := 0; i < len(parSets); i++ {
		par := parSets[i]
		cfgs := []pop.Config{}
		for _, size := range par.Sizes {
			for _, length := range par.Lengths {
				for _, mutation := range par.MutationRates {
					for _, transferInRate := range par.TransferInRates {
						for _, transferInFrag := range par.TransferInFrags {
							for _, transferOutRate := range par.TransferOutRates {
								for _, transferOutFrag := range par.TransferOutFrags {
									cfg := pop.Config{}
									cfg.Size = size
									cfg.Length = length
									cfg.Mutation.Rate = mutation
									cfg.Transfer.In.Rate = transferInRate
									cfg.Transfer.In.Fragment = transferInFrag
									cfg.Transfer.Out.Rate = transferOutRate
									cfg.Transfer.Out.Fragment = transferOutFrag
									cfg.Alphabet = par.Alphabet
									cfgs = append(cfgs, cfg)
								}
							}
						}
					}
				}
			}
		}
		popCfgs = append(popCfgs, cfgs)
	}

	combinations := [][]pop.Config{}
	for i := 0; i < len(popCfgs); i++ {
		set1 := popCfgs[i]
		for j := i + 1; j < len(popCfgs); j++ {
			set2 := popCfgs[j]
			for _, cfg1 := range set1 {
				for _, cfg2 := range set2 {
					combinations = append(combinations, []pop.Config{cfg1, cfg2})
				}
			}
		}
	}

	return combinations
}

func createPops(cfgs []pop.Config) []*pop.Pop {
	// create population generator (with common ancestor).
	genomeSize := cfgs[0].Length
	alphabet := cfgs[0].Alphabet
	ancestor := randomGenerateAncestor(genomeSize, alphabet)
	generator := pop.NewSimplePopGenerator(ancestor)

	pops := make([]*pop.Pop, len(cfgs))
	for i := 0; i < len(pops); i++ {
		pops[i] = cfgs[i].NewPop(generator)
	}

	return pops
}

func randomGenerateAncestor(size int, alphbets []byte) pop.Sequence {
	s := make(pop.Sequence, size)
	for i := 0; i < size; i++ {
		s[i] = alphbets[rand.Intn(len(alphbets))]
	}
	return s
}