package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/sgade/randomorg"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	loggable "github.com/sas1024/gorm-loggable"
)

const (
	//Biden enum
	Biden Candidate = iota
	//Trump enum
	Trump
	//Tie is the rare case there is a tie.
	Tie

	RawPollResults string = "RawPollResults"
	BellCurve      string = "BellCurve"
	CoinFlip       string = "CoinFlip"
	Zeroes         string = "Zeroes"
	RandomOther    string = "RandomOther"
	Proportional   string = "Proportional"
)

//Candidate is the enum for a candidate
type Candidate int

//State contains the individual state data
type State struct {
	State    string
	Trump    float64
	Biden    float64
	Other    float64
	Variance float64
	Electors int
}

//Simulation contains the full election data.
type Simulation struct {
	Polls  Polls
	Runs   int
	Method string
	Key    string
}

type Results struct {
	Biden  int
	Trump  int
	Diff   int
	Winner Candidate

	gorm.Model
	loggable.LoggableModel
}

//Polls is the array of state polls.
type Polls []State

//SimType is the simulation Type used
var SimType = CoinFlip

func main() {

	data, err := ioutil.ReadFile("./polls.json")
	if err != nil {
		panic(err)
	}
	var sim Simulation

	if err = json.Unmarshal([]byte(data), &sim); err != nil {
		panic(err)
	}

	var seed int64
	if sim.Key != "" {
		r := randomorg.NewRandom(sim.Key)

		seeds, err := r.GenerateIntegers(1, 0, 1000000000)
		if err != nil {
			panic(err)
		}
		seed = seeds[0]
	} else {
		seed = time.Now().UnixNano()
	}
	rand.Seed(seed)

	runs := float64(sim.Runs)

	SimType = sim.Method
	trump, biden, tie := sim.Polls.simulate(int(runs))

	tmper := (trump / runs) * 100.0
	biper := (biden / runs) * 100.0
	tiper := (tie / runs) * 100.0
	fmt.Printf("%s RESULTS (SEED: %d)\n"+
		"TOTAL: %05d\n"+
		"TRUMP: %05d (%006.2f %%)\n"+
		"BIDEN: %05d (%006.2f %%)\n"+
		"TIE:   %05d (%006.2f %%)\n",
		SimType, seed,
		int(runs),
		int(trump), tmper,
		int(biden), biper,
		int(tie), tiper)

}

func (polls Polls) simulate(runs int) (trump, biden, tie float64) {

	db, err := gorm.Open("sqlite3", "test.db")
	_, err = loggable.Register(db) // database is a *gorm.DB
	if err != nil {
		panic(err)
	}
	if err != nil {
		panic("failed to connect database")
	}
	defer db.Close()
	db.AutoMigrate(&Results{})

	results := make(map[Candidate]float64)
	var m sync.Mutex
	var wg sync.WaitGroup

	wg.Add(runs)
	for i := 0; i < runs; i++ {
		go func() {
			w := polls.runElection()
			m.Lock()
			db.Create(&w)
			results[w.Winner]++
			wg.Done()
			m.Unlock()

		}()
	}
	wg.Wait()
	return results[Trump], results[Biden], results[Tie]
}

func (polls Polls) runElection() Results {
	trump := 0
	biden := 0
	for _, v := range polls {
		t, b := v.winnerOfState()
		trump += t
		biden += b
	}

	res := Results{Biden: biden, Trump: trump, Diff: int(math.Abs(float64(biden) - float64(trump)))}

	if trump > biden {
		res.Winner = Trump
		return res
	}
	if trump == biden {
		res.Winner = Tie
		return res
	}
	res.Winner = Biden
	return res
}

//Returns the number of electors that each candidate won
func (s State) winnerOfState() (trump, biden int) {
	switch SimType {
	case RawPollResults:
		return s.rawPollResults()
	case BellCurve:
		return s.bellCurve()
	case CoinFlip:
		return s.coinFlip()
	case RandomOther:
		return s.randomOther()
	case Proportional:
		return s.proportional()
	default:
		panic("unknown poll type")
	}

}

//Returns the number of electors that each candidate won
func (s State) proportional() (trump, biden int) {

	trumpPercentage := (s.Trump) / (s.Trump + s.Biden)
	return int(float64(s.Electors) * trumpPercentage), s.Electors - int(float64(s.Electors)*trumpPercentage)

}

//Returns the number of electors that each candidate won
func (s State) randomOther() (trump, biden int) {

	other := rand.Float64() * float64(100-s.Trump-s.Biden)
	if s.Biden+other > s.Trump {
		return 0, s.Electors
	}

	return s.Electors, 0
}

//Returns the number of electors that each candidate won
func (s State) rawPollResults() (trump, biden int) {

	if s.Biden > s.Trump {
		return 0, s.Electors
	}

	if s.Biden < s.Trump {
		return s.Electors, 0
	}

	return 0, 0
}

func (s State) coinFlip() (trump, biden int) {

	value := rand.Float64()

	total := s.Trump + s.Biden
	Needle := (value * total)
	if Needle <= s.Trump {
		return s.Electors, 0
	}
	return 0, s.Electors
}

func (s State) zeroes() (trump, biden int) {

	return 0, 0
}

func (s State) bellCurve() (trump, biden int) {

	dif := s.Variance * lookup(rand.Float64())
	if rand.Float64() >= .5 {
		dif *= -1
	}
	trumpTotal := float64(s.Trump) + dif
	bidenTotal := float64(s.Biden) - dif

	if trumpTotal > bidenTotal {
		return s.Electors, 0
	}
	return 0, s.Electors
}

//returns the number od deviations that the value should change by, based on the
func lookup(z float64) (numStd float64) {

	a := math.Sqrt(1/z - 1)
	b := math.Sqrt(1/z + 1)
	res := math.Log(a*b+(1/z)) / 2.0

	return res
}
