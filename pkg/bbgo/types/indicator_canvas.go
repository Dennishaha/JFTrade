package types

import (
	"time"

	"github.com/wcharczuk/go-chart/v2"
)

type Canvas struct {
	chart.Chart
	Interval Interval
}

func NewCanvas(title string, intervals ...Interval) *Canvas {
	var valueFormatter func(v any) string
	interval := Interval1m
	if len(intervals) > 0 {
		interval = intervals[0]
		if interval.Seconds() > 24*60*60 {
			valueFormatter = chart.TimeDateValueFormatter
		} else if interval.Seconds() > 60*60 {
			valueFormatter = chart.TimeHourValueFormatter
		} else {
			valueFormatter = chart.TimeMinuteValueFormatter
		}
	} else {
		valueFormatter = chart.IntValueFormatter
	}
	out := &Canvas{
		Chart: chart.Chart{
			Title: title,
			XAxis: chart.XAxis{
				ValueFormatter: valueFormatter,
			},
		},
		Interval: interval,
	}
	out.Elements = []chart.Renderable{
		chart.LegendLeft(&out.Chart),
	}
	return out
}

func expand(a []float64, length int, defaultVal float64) []float64 {
	l := len(a)
	if l >= length {
		return a
	}
	for i := 0; i < length-l; i++ {
		a = append([]float64{defaultVal}, a...)
	}
	return a
}

func (canvas *Canvas) Plot(tag string, a Series, endTime Time, length int, intervals ...Interval) {
	var timeline []time.Time
	e := endTime.Time()
	if a.Length() == 0 {
		return
	}
	oldest := a.Last(a.Length() - 1)
	interval := canvas.Interval
	if len(intervals) > 0 {
		interval = intervals[0]
	}
	for i := length - 1; i >= 0; i-- {
		shiftedT := e.Add(-time.Duration(i*interval.Seconds()) * time.Second)
		timeline = append(timeline, shiftedT)
	}
	canvas.Series = append(canvas.Series, chart.TimeSeries{
		Name:    tag,
		YValues: expand(Reverse(a, length), length, oldest),
		XValues: timeline,
	})
}

func (canvas *Canvas) PlotRaw(tag string, a Series, length int) {
	var x []float64
	for i := range length {
		x = append(x, float64(i))
	}
	if a.Length() == 0 {
		return
	}
	oldest := a.Last(a.Length() - 1)
	canvas.Series = append(canvas.Series, chart.ContinuousSeries{
		Name:    tag,
		XValues: x,
		YValues: expand(Reverse(a, length), length, oldest),
	})
}
