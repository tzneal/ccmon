package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"

	"github.com/wcharczuk/go-chart"
	"github.com/wcharczuk/go-chart/drawing"
)

func main() {
	title := flag.String("title", "ccmon", "chart title")

	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}

	series, err := readSeries(flag.Arg(0))
	if err != nil {
		log.Fatalf("reading series, %s", err)
	}

	renderNodesCost(*title, series, "nodes_cost.png")
	renderPendingPodSecondsCost(*title, series, "pending_pods_seconds_cost.png")
}

func renderPendingPodSecondsCost(title string, series *chartData, filename string) {
	var seriesList []chart.Series
	seriesList = append(seriesList, series.cumulativeCost)
	seriesList = append(seriesList, series.pendingPodSeconds)

	f, err := os.Create(filename)
	if err != nil {
		log.Fatalf("opening output, %s", err)
	}
	defer f.Close()
	pendingPodSeconds := chart.Chart{
		Title:      title,
		TitleStyle: chart.Style{Show: true, StrokeColor: drawing.ColorBlack},
		Series:     seriesList,
		XAxis: chart.XAxis{
			Name: "Time",
			Style: chart.Style{
				Show: true,
			},
		},
		YAxis: chart.YAxis{
			Name: "Cost",
			Style: chart.Style{
				Show: true,
			},
		},
		YAxisSecondary: chart.YAxis{
			Name: "Pending Pod Seconds",
			Style: chart.Style{
				Show: true,
			},
		},
	}

	pendingPodSeconds.Elements = []chart.Renderable{
		chart.CreateLegend(&pendingPodSeconds),
	}

	err = pendingPodSeconds.Render(chart.PNG, f)
	if err != nil {
		log.Fatalf("rendering chart, %s", err)
	}
}

func renderNodesCost(title string, series *chartData, filename string) {
	var seriesList []chart.Series
	seriesList = append(seriesList, series.nodes)
	seriesList = append(seriesList, series.cumulativeCost)

	f, err := os.Create(filename)
	if err != nil {
		log.Fatalf("opening output, %s", err)
	}
	defer f.Close()
	nodesAndCost := chart.Chart{
		Title:      title,
		TitleStyle: chart.Style{Show: true, StrokeColor: drawing.ColorBlack},
		Series:     seriesList,
		XAxis: chart.XAxis{
			Name: "Time",
			Style: chart.Style{
				Show: true,
			},
		},
		YAxis: chart.YAxis{
			Name: "Cost",
			Style: chart.Style{
				Show: true,
			},
		},
		YAxisSecondary: chart.YAxis{
			Name: "Node Count",
			Style: chart.Style{
				Show: true,
			},
			ValueFormatter: func(v interface{}) string {
				// nodes are whole numbers
				return strconv.FormatInt(int64(v.(float64)), 10)
			},
		},
	}

	nodesAndCost.Elements = []chart.Renderable{
		chart.CreateLegend(&nodesAndCost),
	}

	err = nodesAndCost.Render(chart.PNG, f)
	if err != nil {
		log.Fatalf("rendering chart, %s", err)
	}
}

type chartData struct {
	nodes             chart.Series
	cumulativeCost    chart.Series
	pendingPodSeconds chart.ContinuousSeries
}

func readSeries(filename string) (*chartData, error) {
	in, err := os.Open(filename)
	if err != nil {
		log.Fatalf("opening output, %s", err)
	}
	defer in.Close()

	cr := csv.NewReader(in)
	header, err := cr.Read()
	if err != nil {
		log.Fatalf("reading header, %s", err)
	}

	indices := map[string]int{}
	for i, hdr := range header {
		indices[hdr] = i
	}
	timeIdx, ok := indices["time"]
	if !ok {
		return nil, fmt.Errorf("unable to find time index")
	}
	nodesIdx, ok := indices["nodes"]
	if !ok {
		return nil, fmt.Errorf("unable to find nodes index")
	}
	cumulativeCostIdx, ok := indices["cumulative cost"]
	if !ok {
		return nil, fmt.Errorf("unable to find cumulative cost index")
	}

	pendingPodSecondsIdx, ok := indices["pending pod seconds"]
	if !ok {
		return nil, fmt.Errorf("unable to find pending pod seconds index")
	}

	nodeSeries := chart.ContinuousSeries{
		Name: "Node Count",
		Style: chart.Style{
			Show: true,
			StrokeColor: drawing.Color{
				B: 255,
				A: 255,
			},
			StrokeWidth: 1.0,
		},
	}
	costSeries := chart.ContinuousSeries{
		Name: "Cost",
		Style: chart.Style{
			Show: true,
			StrokeColor: drawing.Color{
				R: 255,
				A: 255,
			},
			StrokeWidth: 1.0,
		},
	}
	pendingPodSecondsSeries := chart.ContinuousSeries{
		Name: "Pending Pod Seconds",
		Style: chart.Style{
			Show: true,
			StrokeColor: drawing.Color{
				G: 155,
				R: 200,
				A: 255,
			},
			StrokeWidth: 1.0,
		},
	}
	for {
		record, err := cr.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("reading record, %s", err)
		}

		time, err := strconv.ParseFloat(record[timeIdx], 64)
		if err != nil {
			return nil, fmt.Errorf("parsing time, %w", err)
		}

		nodesCount, err := strconv.ParseInt(record[nodesIdx], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parsing nodeSeries, %w", err)
		}

		cost, err := strconv.ParseFloat(record[cumulativeCostIdx], 64)
		if err != nil {
			return nil, fmt.Errorf("parsing cost, %w", err)
		}

		pendingPodSeconds, err := strconv.ParseFloat(record[pendingPodSecondsIdx], 64)
		if err != nil {
			return nil, fmt.Errorf("parsing pending pods, %w", err)
		}

		nodeSeries.XValues = append(nodeSeries.XValues, time)
		nodeSeries.YValues = append(nodeSeries.YValues, float64(nodesCount))
		nodeSeries.YAxis = chart.YAxisSecondary

		pendingPodSecondsSeries.XValues = append(pendingPodSecondsSeries.XValues, time)
		pendingPodSecondsSeries.YValues = append(pendingPodSecondsSeries.YValues, pendingPodSeconds)
		pendingPodSecondsSeries.YAxis = chart.YAxisSecondary

		costSeries.XValues = append(costSeries.XValues, time)
		costSeries.YValues = append(costSeries.YValues, cost)
	}

	return &chartData{
		nodes:             nodeSeries,
		cumulativeCost:    costSeries,
		pendingPodSeconds: pendingPodSecondsSeries,
	}, nil
}
