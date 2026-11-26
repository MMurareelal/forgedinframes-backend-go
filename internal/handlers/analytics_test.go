package handlers

import (
	"testing"

	"google.golang.org/api/analyticsdata/v1beta"
)

func TestParseTopBlogs(t *testing.T) {
	report := &analyticsdata.RunReportResponse{
		Rows: []*analyticsdata.Row{
			{
				DimensionValues: []*analyticsdata.DimensionValue{
					{Value: "/journal/Optical%20Reflections"},
					{Value: "Optical Reflections | ForgedInFrames"},
				},
				MetricValues: []*analyticsdata.MetricValue{
					{Value: "10"},
					{Value: "650"},
				},
			},
			{
				DimensionValues: []*analyticsdata.DimensionValue{
					{Value: "/journal/Optical%20Reflections"},
					{Value: "ForgedInFrames"},
				},
				MetricValues: []*analyticsdata.MetricValue{
					{Value: "5"},
					{Value: "250"},
				},
			},
			{
				DimensionValues: []*analyticsdata.DimensionValue{
					{Value: "/journal/monsoon-frames"},
					{Value: "(not set)"},
				},
				MetricValues: []*analyticsdata.MetricValue{
					{Value: "3"},
					{Value: "90"},
				},
			},
			{
				DimensionValues: []*analyticsdata.DimensionValue{
					{Value: "/journal/"},
					{Value: "Journal Index"},
				},
				MetricValues: []*analyticsdata.MetricValue{
					{Value: "20"},
					{Value: "400"},
				},
			},
		},
	}

	got := parseTopBlogs(report)
	if len(got) != 2 {
		t.Fatalf("expected 2 journal entries, got %d", len(got))
	}

	if got[0].ID != "Optical Reflections" || got[0].Title != "Optical Reflections" || got[0].Reads != 15 || got[0].AvgReadTime != "1m 0s" {
		t.Errorf("unexpected first entry: %+v", got[0])
	}

	if got[1].ID != "monsoon-frames" || got[1].Title != "monsoon-frames" || got[1].Reads != 3 || got[1].AvgReadTime != "0m 30s" {
		t.Errorf("unexpected second entry: %+v", got[1])
	}
}
