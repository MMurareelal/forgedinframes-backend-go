package handlers

import (
	"context"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/api/analyticsdata/v1beta"
	"google.golang.org/api/option"

	"forgedinframes-backend-go/config"
	fb "forgedinframes-backend-go/internal/firebase"
	"forgedinframes-backend-go/internal/models"
)

type AnalyticsHandler struct {
	cfg      *config.Config
	fbClient *fb.Client
}

func NewAnalyticsHandler(cfg *config.Config, fbClient *fb.Client) *AnalyticsHandler {
	return &AnalyticsHandler{
		cfg:      cfg,
		fbClient: fbClient,
	}
}

// createAnalyticsService instantiates the Google Analytics Data API client
func (h *AnalyticsHandler) createAnalyticsService(ctx context.Context) (*analyticsdata.Service, error) {
	var opts []option.ClientOption
	if h.fbClient != nil && len(h.fbClient.ClientOptions) > 0 {
		opts = append(opts, h.fbClient.ClientOptions...)
	}
	opts = append(opts, option.WithScopes(analyticsdata.AnalyticsReadonlyScope))

	return analyticsdata.NewService(ctx, opts...)
}

// GetGA4Summary handles GET /api/analytics/ga4/summary
// Fetches real telemetry from Google Analytics 4 via the Google Analytics Data API
func (h *AnalyticsHandler) GetGA4Summary(c *gin.Context) {
	timeRange := c.DefaultQuery("timeRange", "7d")
	var startDate string
	switch timeRange {
	case "24h":
		startDate = "1daysAgo"
	case "7d":
		startDate = "7daysAgo"
	case "30d":
		startDate = "30daysAgo"
	case "all":
		startDate = "365daysAgo"
	default:
		timeRange = "7d"
		startDate = "7daysAgo"
	}

	cleanPropertyID := strings.TrimPrefix(strings.TrimSpace(h.cfg.GA4PropertyID), "properties/")
	if cleanPropertyID == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"connected": false,
			"source":    "client_fallback",
			"error":     "GA4_PROPERTY_ID is not configured on backend",
		})
		return
	}

	targetProperty := fmt.Sprintf("properties/%s", cleanPropertyID)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	srv, err := h.createAnalyticsService(ctx)
	if err != nil {
		log.Printf("Analytics client initialization error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"connected": false,
			"source":    "client_fallback",
			"error":     fmt.Sprintf("Failed to initialize Analytics Data client: %v", err),
		})
		return
	}

	var (
		wg             sync.WaitGroup
		coreReport     *analyticsdata.RunReportResponse
		coreErr        error
		timeReport     *analyticsdata.RunReportResponse
		timeErr        error
		deviceReport   *analyticsdata.RunReportResponse
		deviceErr      error
		browserReport  *analyticsdata.RunReportResponse
		browserErr     error
		referrerReport *analyticsdata.RunReportResponse
		referrerErr    error
	)

	// 1. Core KPIs Report
	wg.Add(1)
	go func() {
		defer wg.Done()
		coreReport, coreErr = srv.Properties.RunReport(targetProperty, &analyticsdata.RunReportRequest{
			DateRanges: []*analyticsdata.DateRange{
				{StartDate: startDate, EndDate: "today"},
			},
			Metrics: []*analyticsdata.Metric{
				{Name: "screenPageViews"},
				{Name: "activeUsers"},
				{Name: "userEngagementDuration"},
			},
		}).Context(ctx).Do()
	}()

	// 2. Daily Timeline
	wg.Add(1)
	go func() {
		defer wg.Done()
		timeReport, timeErr = srv.Properties.RunReport(targetProperty, &analyticsdata.RunReportRequest{
			DateRanges: []*analyticsdata.DateRange{
				{StartDate: startDate, EndDate: "today"},
			},
			Dimensions: []*analyticsdata.Dimension{
				{Name: "date"},
			},
			Metrics: []*analyticsdata.Metric{
				{Name: "screenPageViews"},
				{Name: "activeUsers"},
			},
			OrderBys: []*analyticsdata.OrderBy{
				{
					Dimension: &analyticsdata.DimensionOrderBy{
						DimensionName: "date",
					},
				},
			},
		}).Context(ctx).Do()
	}()

	// 3. Device Category Breakdown
	wg.Add(1)
	go func() {
		defer wg.Done()
		deviceReport, deviceErr = srv.Properties.RunReport(targetProperty, &analyticsdata.RunReportRequest{
			DateRanges: []*analyticsdata.DateRange{
				{StartDate: startDate, EndDate: "today"},
			},
			Dimensions: []*analyticsdata.Dimension{
				{Name: "deviceCategory"},
			},
			Metrics: []*analyticsdata.Metric{
				{Name: "activeUsers"},
			},
		}).Context(ctx).Do()
	}()

	// 4. Browser Breakdown
	wg.Add(1)
	go func() {
		defer wg.Done()
		browserReport, browserErr = srv.Properties.RunReport(targetProperty, &analyticsdata.RunReportRequest{
			DateRanges: []*analyticsdata.DateRange{
				{StartDate: startDate, EndDate: "today"},
			},
			Dimensions: []*analyticsdata.Dimension{
				{Name: "browser"},
			},
			Metrics: []*analyticsdata.Metric{
				{Name: "activeUsers"},
			},
			Limit: 10,
		}).Context(ctx).Do()
	}()

	// 5. Referrer Breakdown
	wg.Add(1)
	go func() {
		defer wg.Done()
		referrerReport, referrerErr = srv.Properties.RunReport(targetProperty, &analyticsdata.RunReportRequest{
			DateRanges: []*analyticsdata.DateRange{
				{StartDate: startDate, EndDate: "today"},
			},
			Dimensions: []*analyticsdata.Dimension{
				{Name: "sessionSource"},
			},
			Metrics: []*analyticsdata.Metric{
				{Name: "activeUsers"},
			},
			Limit: 10,
		}).Context(ctx).Do()
	}()

	wg.Wait()

	if coreErr != nil {
		log.Printf("GA4 API Core Report error: %v", coreErr)
		c.JSON(http.StatusInternalServerError, gin.H{
			"connected": false,
			"source":    "client_fallback",
			"error":     coreErr.Error(),
		})
		return
	}

	// Parse Core Metrics
	var totalViews, uniqueVisitors, totalEngagementSec int
	if coreReport != nil && len(coreReport.Rows) > 0 && len(coreReport.Rows[0].MetricValues) >= 3 {
		totalViews, _ = strconv.Atoi(coreReport.Rows[0].MetricValues[0].Value)
		uniqueVisitors, _ = strconv.Atoi(coreReport.Rows[0].MetricValues[1].Value)
		totalEngagementSec, _ = strconv.Atoi(coreReport.Rows[0].MetricValues[2].Value)
	}

	avgDwellSeconds := 0
	if uniqueVisitors > 0 {
		avgDwellSeconds = int(math.Round(float64(totalEngagementSec) / float64(uniqueVisitors)))
	}

	// Parse Time Series
	timeSeries := make([]models.AnalyticsTimeSeriesPoint, 0)
	if timeErr == nil && timeReport != nil {
		for _, row := range timeReport.Rows {
			if len(row.DimensionValues) > 0 && len(row.MetricValues) >= 2 {
				rawDate := row.DimensionValues[0].Value
				dateLabel := rawDate
				if len(rawDate) == 8 {
					dateLabel = fmt.Sprintf("%s/%s", rawDate[4:6], rawDate[6:8])
				}
				views, _ := strconv.Atoi(row.MetricValues[0].Value)
				visitors, _ := strconv.Atoi(row.MetricValues[1].Value)

				timeSeries = append(timeSeries, models.AnalyticsTimeSeriesPoint{
					Date:     dateLabel,
					Views:    views,
					Visitors: visitors,
				})
			}
		}
	}

	// Parse Device Breakdown
	var desktop, mobile, tablet int
	if deviceErr == nil && deviceReport != nil {
		for _, row := range deviceReport.Rows {
			if len(row.DimensionValues) > 0 && len(row.MetricValues) > 0 {
				cat := strings.ToLower(row.DimensionValues[0].Value)
				count, _ := strconv.Atoi(row.MetricValues[0].Value)
				switch cat {
				case "mobile":
					mobile += count
				case "tablet":
					tablet += count
				default:
					desktop += count
				}
			}
		}
	}

	totalDev := desktop + mobile + tablet
	if totalDev == 0 {
		totalDev = 1
	}

	deviceBreakdown := models.AnalyticsDeviceBreakdown{
		Desktop: int(math.Round(float64(desktop) / float64(totalDev) * 100)),
		Mobile:  int(math.Round(float64(mobile) / float64(totalDev) * 100)),
		Tablet:  int(math.Round(float64(tablet) / float64(totalDev) * 100)),
	}

	// Parse Browser Breakdown
	browserBreakdown := make([]models.AnalyticsBrowserBreakdown, 0)
	if browserErr == nil && browserReport != nil {
		var totalBrowserUsers int
		for _, row := range browserReport.Rows {
			if len(row.MetricValues) > 0 {
				u, _ := strconv.Atoi(row.MetricValues[0].Value)
				totalBrowserUsers += u
			}
		}
		if totalBrowserUsers > 0 {
			for _, row := range browserReport.Rows {
				if len(row.DimensionValues) > 0 && len(row.MetricValues) > 0 {
					bName := row.DimensionValues[0].Value
					u, _ := strconv.Atoi(row.MetricValues[0].Value)
					browserBreakdown = append(browserBreakdown, models.AnalyticsBrowserBreakdown{
						Browser:    bName,
						Percentage: int(math.Round(float64(u) / float64(totalBrowserUsers) * 100)),
					})
				}
			}
		}
	}

	// Parse Referrer Breakdown
	referrerBreakdown := make([]models.AnalyticsReferrerBreakdown, 0)
	if referrerErr == nil && referrerReport != nil {
		var totalRefUsers int
		for _, row := range referrerReport.Rows {
			if len(row.MetricValues) > 0 {
				u, _ := strconv.Atoi(row.MetricValues[0].Value)
				totalRefUsers += u
			}
		}
		if totalRefUsers > 0 {
			for _, row := range referrerReport.Rows {
				if len(row.DimensionValues) > 0 && len(row.MetricValues) > 0 {
					src := row.DimensionValues[0].Value
					u, _ := strconv.Atoi(row.MetricValues[0].Value)
					referrerBreakdown = append(referrerBreakdown, models.AnalyticsReferrerBreakdown{
						Source:     src,
						Count:      u,
						Percentage: int(math.Round(float64(u) / float64(totalRefUsers) * 100)),
					})
				}
			}
		}
	}

	// Construct response matching exact specification
	response := models.GA4SummaryResponse{
		Connected:  true,
		Source:     "ga4",
		PropertyID: cleanPropertyID,
		Summary: models.AnalyticsSummary{
			TotalViews:             totalViews,
			UniqueVisitors:         uniqueVisitors,
			AvgDwellSeconds:        avgDwellSeconds,
			TotalPhotoInteractions: 0,
			TimeRange:              timeRange,
			ViewsTrend:             0,
			VisitorsTrend:          0,
			InteractionsTrend:      0,
			TimeSeries:             timeSeries,
			CategoryBreakdown:      make([]models.AnalyticsCategoryBreakdown, 0),
			TopPhotos:              make([]models.AnalyticsTopPhoto, 0),
			TopBlogs:               make([]models.AnalyticsTopBlog, 0),
			DeviceBreakdown:        deviceBreakdown,
			BrowserBreakdown:       browserBreakdown,
			ReferrerBreakdown:      referrerBreakdown,
			RecentEvents:           make([]models.AnalyticsEvent, 0),
		},
	}

	c.JSON(http.StatusOK, response)
}
