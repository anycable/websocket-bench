package benchmark

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

type ResultRecorder interface {
	Record(
		clientCount int,
		limitPercentile int,
		rttPercentile time.Duration,
		rttMin time.Duration,
		rttMedian time.Duration,
		rttMax time.Duration,
	) error
	Message(str string)
	Flush() error
}

type JSONResultRecorder struct {
	w        io.Writer
	messages []string
	records  []map[string]interface{}
}

func NewJSONResultRecorder(w io.Writer) *JSONResultRecorder {
	return &JSONResultRecorder{w: w}
}

func (jrr *JSONResultRecorder) Record(
	clientCount, limitPercentile int,
	rttPercentile, rttMin, rttMedian, rttMax time.Duration,
) error {
	record := map[string]interface{}{
		"time":       time.Now().Format(time.RFC3339),
		"clients":    clientCount,
		"limit_per":  limitPercentile,
		"per-rtt":    roundToMS(rttPercentile),
		"min-rtt":    roundToMS(rttMin),
		"median-rtt": roundToMS(rttMedian),
		"max-rtt":    roundToMS(rttMax),
	}

	jrr.records = append(jrr.records, record)

	return nil
}

func (jrr *JSONResultRecorder) Message(str string) {
	jrr.messages = append(jrr.messages, str)
}

func (jrr *JSONResultRecorder) Flush() error {
	res := map[string]interface{}{"steps": jrr.records, "messages": jrr.messages}
	jsonString, err := json.Marshal(res)
	if err != nil {
		return err
	}

	if _, err := jrr.w.Write(jsonString); err != nil {
		return err
	}

	return nil
}

type TextResultRecorder struct {
	w io.Writer
}

func NewTextResultRecorder(w io.Writer) *TextResultRecorder {
	return &TextResultRecorder{w: w}
}

func (trr *TextResultRecorder) Flush() error {
	// Do nothing
	return nil
}

func (trr *TextResultRecorder) Message(str string) {
	fmt.Println(str)
}

func (trr *TextResultRecorder) Record(
	clientCount, limitPercentile int,
	rttPercentile, rttMin, rttMedian, rttMax time.Duration,
) error {
	_, err := fmt.Fprintf(trr.w,
		"[%s] clients: %5d    %dper-rtt: %3dms    min-rtt: %3dms    median-rtt: %3dms    max-rtt: %3dms\n",
		time.Now().Format(time.RFC3339),
		clientCount,
		limitPercentile,
		roundToMS(rttPercentile),
		roundToMS(rttMin),
		roundToMS(rttMedian),
		roundToMS(rttMax),
	)

	return err
}

func roundToMS(d time.Duration) int64 {
	return int64((d + (500 * time.Microsecond)) / time.Millisecond)
}
