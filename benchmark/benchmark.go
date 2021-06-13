package benchmark

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/cheggaaa/pb/v3"
)

const (
	ConnectionTimeout = 5 * time.Minute
)

type Benchmark struct {
	errChan       chan error
	rttResultChan chan time.Duration

	payloadPadding []byte

	Config

	clients []Client
}

type Config struct {
	WebsocketURL          string
	WebsocketOrigin       string
	WebsocketProtocol     string
	ServerType            string
	ClientCmd             int
	PayloadPaddingSize    int
	InitialClients        int
	StepSize              int
	Concurrent            int
	ConcurrentConnect     int
	SampleSize            int
	LimitPercentile       int
	LimitRTT              time.Duration
	TotalSteps            int
	Interactive           bool
	StepDelay             time.Duration
	CommandDelay          time.Duration
	CommandDelayChance    int
	WaitBroadcastsSeconds int
	ClientPools           []ClientPool
	ResultRecorder        ResultRecorder
}

func New(config *Config) *Benchmark {
	b := &Benchmark{Config: *config}

	b.errChan = make(chan error)
	b.rttResultChan = make(chan time.Duration)

	b.payloadPadding = []byte(strings.Repeat(
		"1234567890",
		b.PayloadPaddingSize/10+1,
	)[:b.PayloadPaddingSize])

	return b
}

func (b *Benchmark) Run() error {
	var expectedRxBroadcastCount int

	if b.InitialClients == 0 {
		b.InitialClients = b.StepSize
	}

	b.startClients(b.ServerType, b.InitialClients, b.ConcurrentConnect)

	stepNum := 0
	drop := 0

	finished := false

	for {

		stepNum++

		stepDrop := 0

		bar := pb.StartNew(b.SampleSize)

		inProgress := 0
		for i := 0; i < b.Concurrent; i++ {
			if err := b.sendToRandomClient(); err != nil {
				return err
			}
			inProgress++
		}

		var rttAgg rttAggregate
		for rttAgg.Count()+stepDrop < b.SampleSize {
			select {
			case result := <-b.rttResultChan:
				rttAgg.Add(result)
				bar.Increment()
				inProgress--
			case err := <-b.errChan:
				stepDrop++
				debug(fmt.Sprintf("error: %v", err))
			}

			if rttAgg.Count()+inProgress+stepDrop < b.SampleSize {
				if err := b.sendToRandomClient(); err != nil {
					return err
				}
				inProgress++
			}
		}

		bar.Finish()

		drop += stepDrop

		expectedRxBroadcastCount += (len(b.clients) - drop) * b.SampleSize

		if (b.TotalSteps > 0 && b.TotalSteps == stepNum) || (b.TotalSteps == 0 && b.LimitRTT < rttAgg.Percentile(b.LimitPercentile)) {
			finished = true

			if b.ClientCmd == ClientBroadcastCmd {
				// Due to the async nature of the broadcasts and the receptions, it is
				// possible for the broadcastResult to arrive before all the
				// broadcasts. This isn't really a problem when running the benchmark
				// because the samples are will balance each other out. However, it
				// does matter when checking at the end that all expected broadcasts
				// were received. So we wait a little before getting the broadcast
				// count.
				time.Sleep(time.Duration(b.WaitBroadcastsSeconds) * time.Second)

				totalRxBroadcastCount := 0
				for _, c := range b.clients {
					count, err := c.ResetRxBroadcastCount()
					if err != nil {
						return err
					}
					totalRxBroadcastCount += count
				}
				if totalRxBroadcastCount < expectedRxBroadcastCount {
					b.ResultRecorder.Message(
						fmt.Sprintf("Missing received broadcasts: expected %d, got %d", expectedRxBroadcastCount, totalRxBroadcastCount),
					)
				}
				if totalRxBroadcastCount > expectedRxBroadcastCount {
					b.ResultRecorder.Message(
						fmt.Sprintf("Extra received broadcasts: expected %d, got %d", expectedRxBroadcastCount, totalRxBroadcastCount),
					)
				}
			}
		}

		err := b.ResultRecorder.Record(
			len(b.clients)-drop,
			b.LimitPercentile,
			rttAgg.Percentile(b.LimitPercentile),
			rttAgg.Min(),
			rttAgg.Percentile(50),
			rttAgg.Max(),
		)
		if err != nil {
			return err
		}

		if finished {
			return nil
		}

		if b.Interactive {
			promptToContinue()
		}

		if b.StepDelay > 0 {
			time.Sleep(b.StepDelay)
		}

		b.startClients(b.ServerType, b.StepSize, b.ConcurrentConnect)
	}
}

func (b *Benchmark) startClients(serverType string, total int, concurrent int) {
	bar := pb.Simple.Start(total)
	created := 0
	counter := len(b.clients)

	for created < total {
		var waitgroup sync.WaitGroup
		var mu sync.Mutex

		toCreate := int(math.Min(float64(concurrent), float64(total-created)))

		for i := 0; i < toCreate; i++ {
			waitgroup.Add(1)

			go func() {
				cp := b.ClientPools[i%len(b.ClientPools)]
				client, err := cp.New(counter, b.WebsocketURL, b.WebsocketOrigin, b.ServerType, b.rttResultChan, b.errChan, b.payloadPadding)

				if err != nil {
					b.errChan <- err
				}
				mu.Lock()
				b.clients = append(b.clients, client)
				bar.Increment()
				mu.Unlock()
				waitgroup.Done()
			}()
		}
		waitgroup.Wait()
		created += toCreate
	}

	bar.Finish()
}

func (b *Benchmark) randomClient() Client {
	if len(b.clients) == 0 {
		panic("no clients")
	}

	return b.clients[rand.Intn(len(b.clients))]
}

func (b *Benchmark) sendToRandomClient() error {
	if len(b.clients) == 0 {
		panic("no clients")
	}

	if b.CommandDelay > 0 && b.CommandDelayChance > rand.Intn(100) {
		time.Sleep(b.CommandDelay)
	}

	client := b.randomClient()
	switch b.ClientCmd {
	case ClientEchoCmd:
		if err := client.SendEcho(); err != nil {
			return err
		}
	case ClientBroadcastCmd:
		if err := client.SendBroadcast(); err != nil {
			return err
		}
	default:
		panic("unknown client command")
	}

	return nil
}

func promptToContinue() {
	fmt.Print("Press any key to continue to the next step")
	var input string
	fmt.Scanln(&input)
}

func printNow(label string) {
	fmt.Printf("[%s] %s\n", time.Now().Format(time.RFC3339), label)
}

func debug(msg string) {
	fmt.Printf("DEBUG [%s] %s\n", time.Now().Format(time.RFC3339), msg)
}
