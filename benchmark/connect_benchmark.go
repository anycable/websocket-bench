package benchmark

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cheggaaa/pb/v3"
)

type ConnectBenchmark struct {
	errChan chan error
	resChan chan time.Duration

	Config

	clientsCount uint64
}

func NewConnect(config *Config) *ConnectBenchmark {
	b := &ConnectBenchmark{Config: *config}

	b.errChan = make(chan error)
	b.resChan = make(chan time.Duration)

	return b
}

func (b *ConnectBenchmark) Run() error {
	stepNum := 0
	drop := 0

	for {

		stepDrop := 0

		stepNum++

		bar := pb.StartNew(b.StepSize)

		go b.startClients(b.Concurrent, b.StepSize)

		var resAgg rttAggregate
		for resAgg.Count()+stepDrop < b.StepSize {
			select {
			case result := <-b.resChan:
				bar.Increment()
				resAgg.Add(result)
			case err := <-b.errChan:
				fmt.Printf("drop %v", err)
				stepDrop++
				bar.Increment()
			}
		}

		bar.Finish()

		drop += stepDrop

		err := b.ResultRecorder.Record(
			int(b.clientsCount)-drop,
			b.LimitPercentile,
			resAgg.Percentile(b.LimitPercentile),
			resAgg.Min(),
			resAgg.Percentile(50),
			resAgg.Max(),
		)

		if err != nil {
			return err
		}

		if b.TotalSteps > 0 && b.TotalSteps == stepNum {
			return nil
		}

		if b.Interactive {
			promptToContinue()
		}

		if b.StepDelay > 0 {
			time.Sleep(b.StepDelay)
		}
	}
}

func (b *ConnectBenchmark) startClient(serverType string) error {
	cp := b.ClientPools[int(b.clientsCount)%len(b.ClientPools)]

	if b.CommandDelay > 0 && b.CommandDelayChance > rand.Intn(100) {
		time.Sleep(b.CommandDelay)
	}

	atomic.AddUint64(&b.clientsCount, 1)
	_, err := cp.New(int(b.clientsCount), b.WebsocketURL, b.WebsocketOrigin, b.ServerType, b.resChan, b.errChan, nil)

	if err != nil {
		return err
	}

	return nil
}

func (b *ConnectBenchmark) startClients(num int, total int) {
	created := 0

	for created < total {
		var waitgroup sync.WaitGroup

		toCreate := int(math.Min(float64(num), float64(total-created)))

		for i := 0; i < toCreate; i++ {
			waitgroup.Add(1)

			go func() {
				if err := b.startClient(b.ServerType); err != nil {
					b.errChan <- err
				}
				waitgroup.Done()
			}()
		}
		waitgroup.Wait()
		created += toCreate
	}
}

func printNow(label string) {
	fmt.Printf("[%s] %s\n", time.Now(), label)
}
