package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestRunSameDay20DaysAgo(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer closeDB(db)

	logPath := filepath.Join(tempDir, "access.log")

	testLogLines := []string{
		fmt.Sprintf(`192.168.1.1 - - [%s] "GET /index.html HTTP/1.1" 200 1234 "https://google.com" "Mozilla/5.0" "example.com"`, time.Now().AddDate(0, 0, -20).Format("02/Jan/2006:15:04:05 -0700")),
		fmt.Sprintf(`10.0.0.5 - - [%s] "GET /api/data HTTP/1.1" 200 5678 "-" "curl/7.68.0" "example.com"`, time.Now().Add(15*time.Second).AddDate(0, 0, -20).Format("02/Jan/2006:15:04:05 -0700")),
		fmt.Sprintf(`192.168.1.100 - - [%s] "GET /style.css HTTP/1.1" 200 900 "https://example.com" "Mozilla/5.0" "example.com"`, time.Now().Add(15*time.Second).AddDate(0, 0, -20).Format("02/Jan/2006:15:04:05 -0700")),
	}
	createTestLogFile(t, logPath, testLogLines)

	pageViews := make(chan PageView, 100)

	var wg sync.WaitGroup
	wg.Add(1)

	go processPageviewsWithWaitGroup(db, pageViews, &wg)

	tailArgs := []string{"-n", "+1", logPath}
	tailLog(tailArgs, pageViews)
	close(pageViews)

	wg.Wait()

	visitorHashesQuery := `SELECT * FROM visitor_hashes`
	visitorRows, err := db.Query(visitorHashesQuery)
	if err != nil {
		t.Fatalf("could not query the database for vistor hashes: %v", err)
	}
	defer visitorRows.Close()
	var visitorHashes []VisitorHash
	for visitorRows.Next() {
		var visitorHash VisitorHash
		err := visitorRows.Scan(
			&visitorHash.Hash,
			&visitorHash.HourBucket,
			&visitorHash.FirstSeen,
		)
		if err != nil {
			t.Fatalf("unable to parse database visitor hash output, %v", err)
		}
		visitorHashes = append(visitorHashes, visitorHash)
	}

	if len(visitorHashes) != 3 {
		t.Errorf("Expected 3 entries in visitor hash table, got %d instead", len(visitorHashes))
	}

	// allInSameHourBucket := true
	// hourBucket := 0
	// for index, visitorHash := range visitorHashes {
	// 	if index == 0 {
	// 		hourBucket = visitorHash.HourBucket
	// 		continue
	// 	} else if visitorHash.HourBucket != hourBucket {
	// 		allInSameHourBucket = false
	// 		break
	// 	}
	// }
	// if !allInSameHourBucket {
	// 	t.Errorf("Expected all the entries to be in the same hour bucket")
	// }

	hourlyStatsQuery := `SELECT * FROM hourly_stats`
	hourlyStatsRows, err := db.Query(hourlyStatsQuery)
	if err != nil {
		t.Fatalf("could not query the database for hourly stats: %v", err)
	}
	defer hourlyStatsRows.Close()
	var hourlyStats []HourlyStats
	for hourlyStatsRows.Next() {
		var hourlyStat HourlyStats
		err := hourlyStatsRows.Scan(
			&hourlyStat.Hour,
			&hourlyStat.YearDay,
			&hourlyStat.Year,
			&hourlyStat.Path,
			&hourlyStat.Host,
			&hourlyStat.Pageviews,
			&hourlyStat.UniqueVisitors,
			&hourlyStat.BotViews,
		)
		if err != nil {
			t.Fatalf("unable to parse database hourly stat output, %v", err)
		}
		hourlyStats = append(hourlyStats, hourlyStat)
	}
	if len(hourlyStats) != 3 {
		t.Errorf("Expected 3 entries in hourly stats table, got %d instead", len(hourlyStats))
	}

	if hourlyStats[2].UniqueVisitors != 3 {
		t.Errorf("Expected 3 unique visitors in hourly stats table, got %d instead", hourlyStats[2].UniqueVisitors)
	}

	if hourlyStats[0].Pageviews != 1 {
		t.Errorf("Expected first entry to have 1 page view as count, got %d instead\n", hourlyStats[0].Pageviews)
	}
	if hourlyStats[0].BotViews != 0 {
		t.Errorf("Expected first entry to have 0 bot view as count, got %d instead\n", hourlyStats[0].BotViews)
	}

	if hourlyStats[1].Pageviews != 0 {
		t.Errorf("Expected second entry to have 0 page view as count, got %d instead\n", hourlyStats[1].Pageviews)
	}
	if hourlyStats[1].BotViews != 1 {
		t.Errorf("Expected second entry to have 1 bot view as count, got %d instead\n", hourlyStats[1].BotViews)
	}

	if hourlyStats[2].Pageviews != 1 {
		t.Errorf("Expected third entry to have 1 page view as count, got %d instead\n", hourlyStats[2].Pageviews)
	}
	if hourlyStats[2].BotViews != 0 {
		t.Errorf("Expected third entry to have 0 bot view as count, got %d instead\n", hourlyStats[2].BotViews)
	}

	hourlyStatusCodesQuery := `SELECT * FROM hourly_status_codes`
	hourlyStatusCodesRows, err := db.Query(hourlyStatusCodesQuery)
	if err != nil {
		t.Fatalf("could not query the database for hourly status codes: %v", err)
	}
	defer hourlyStatusCodesRows.Close()
	var hourlyStatusCodes []HourlyStatusCodes
	for hourlyStatusCodesRows.Next() {
		var hourlyStatusCode HourlyStatusCodes
		err := hourlyStatusCodesRows.Scan(
			&hourlyStatusCode.Hour,
			&hourlyStatusCode.YearDay,
			&hourlyStatusCode.Year,
			&hourlyStatusCode.Path,
			&hourlyStatusCode.Host,
			&hourlyStatusCode.StatusCode,
			&hourlyStatusCode.Count,
		)
		if err != nil {
			t.Fatalf("unable to parse database hourly status code output, %v", err)
		}
		hourlyStatusCodes = append(hourlyStatusCodes, hourlyStatusCode)
	}
	if len(hourlyStatusCodes) != 3 {
		t.Errorf("Expected 3 entries in hourly status codes table, got %d instead", len(hourlyStatusCodes))
	}

	all200StatusCodes := true
	for _, hourlyStatusCode := range hourlyStatusCodes {
		if hourlyStatusCode.StatusCode != 200 {
			all200StatusCodes = false
			break
		}
	}
	if !all200StatusCodes {
		t.Errorf("Expected all the entries to have 200 status code")
	}

	hourlyReferrersQuery := `SELECT * FROM hourly_referrers`
	hourlyReferrersRows, err := db.Query(hourlyReferrersQuery)
	if err != nil {
		t.Fatalf("could not query the database for hourly referrers: %v", err)
	}
	defer hourlyReferrersRows.Close()
	var hourlyReferrers []HourlyReferrers
	for hourlyReferrersRows.Next() {
		var hourlyReferrer HourlyReferrers
		err := hourlyReferrersRows.Scan(
			&hourlyReferrer.Hour,
			&hourlyReferrer.YearDay,
			&hourlyReferrer.Year,
			&hourlyReferrer.Path,
			&hourlyReferrer.Host,
			&hourlyReferrer.Referrer,
			&hourlyReferrer.Count,
		)
		if err != nil {
			t.Fatalf("unable to parse database hourly referrers output, %v", err)
		}
		hourlyReferrers = append(hourlyReferrers, hourlyReferrer)
	}
	if len(hourlyReferrers) != 3 {
		t.Errorf("Expected 3 entries in hourly referrers table, got %d instead", len(hourlyReferrers))
	}

	allReferrerCountIsOne := true
	for _, hourlyReferrer := range hourlyReferrers {
		if hourlyReferrer.Count != 1 {
			allReferrerCountIsOne = false
			break
		}
	}
	if !allReferrerCountIsOne {
		t.Errorf("Expected all the entries to have 1 as count")
	}

	ticker := time.NewTicker(1 * time.Second)
	shutdown := make(chan os.Signal, 1)

	wg.Add(1)
	go runPeriodicCleanupsWithWaitGroup(db, ticker, shutdown, &wg)

	shutdown <- syscall.SIGTERM
	close(shutdown)
	wg.Wait()

	var hourly_stats_count int
	err = db.QueryRow("SELECT COUNT(*) FROM hourly_stats").Scan(&hourly_stats_count)
	if err != nil {
		t.Fatalf("Failed to count records: %v", err)
	}

	if hourly_stats_count != 3 {
		t.Errorf("Expected 3 hourly_stats record remaining, got %d", hourly_stats_count)
	}

	var hourly_status_codes_count int
	err = db.QueryRow("SELECT COUNT(*) FROM hourly_status_codes").Scan(&hourly_status_codes_count)
	if err != nil {
		t.Fatalf("Failed to count records: %v", err)
	}

	if hourly_status_codes_count != 3 {
		t.Errorf("Expected 3 hourly_status_codes record remaining, got %d", hourly_status_codes_count)
	}

	var hourly_referrers_count int
	err = db.QueryRow("SELECT COUNT(*) FROM hourly_referrers").Scan(&hourly_referrers_count)
	if err != nil {
		t.Fatalf("Failed to count records: %v", err)
	}

	if hourly_referrers_count != 3 {
		t.Errorf("Expected 3 hourly_referrers record remaining, got %d", hourly_referrers_count)
	}

	var visitor_hashes_count int
	err = db.QueryRow("SELECT COUNT(*) FROM visitor_hashes").Scan(&visitor_hashes_count)
	if err != nil {
		t.Fatalf("Failed to count records: %v", err)
	}

	if visitor_hashes_count != 0 {
		t.Errorf("Expected 0 visitor_hashes record remaining, got %d", visitor_hashes_count)
	}
}

func TestRunSameDayInThePast(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer closeDB(db)

	logPath := filepath.Join(tempDir, "access.log")

	testLogLines := []string{
		`192.168.1.1 - - [24/Dec/2024:10:30:45 +0000] "GET /index.html HTTP/1.1" 200 1234 "https://google.com" "Mozilla/5.0" "example.com"`,
		`10.0.0.5 - - [24/Dec/2024:10:31:00 +0000] "GET /api/data HTTP/1.1" 200 5678 "-" "curl/7.68.0" "example.com"`,
		`192.168.1.100 - - [24/Dec/2024:10:31:15 +0000] "GET /style.css HTTP/1.1" 200 900 "https://example.com" "Mozilla/5.0" "example.com"`,
	}
	createTestLogFile(t, logPath, testLogLines)

	pageViews := make(chan PageView, 100)

	var wg sync.WaitGroup
	wg.Add(1)

	go processPageviewsWithWaitGroup(db, pageViews, &wg)

	tailArgs := []string{"-n", "+1", logPath}
	tailLog(tailArgs, pageViews)
	close(pageViews)

	wg.Wait()

	visitorHashesQuery := `SELECT * FROM visitor_hashes`
	visitorRows, err := db.Query(visitorHashesQuery)
	if err != nil {
		t.Fatalf("could not query the database for vistor hashes: %v", err)
	}
	defer visitorRows.Close()
	var visitorHashes []VisitorHash
	for visitorRows.Next() {
		var visitorHash VisitorHash
		err := visitorRows.Scan(
			&visitorHash.Hash,
			&visitorHash.HourBucket,
			&visitorHash.FirstSeen,
		)
		if err != nil {
			t.Fatalf("unable to parse database visitor hash output, %v", err)
		}
		visitorHashes = append(visitorHashes, visitorHash)
	}

	if len(visitorHashes) != 3 {
		t.Errorf("Expected 3 entries in visitor hash table, got %d instead", len(visitorHashes))
	}

	allInHourBucket10 := true
	for _, visitorHash := range visitorHashes {
		if visitorHash.HourBucket != 10 {
			allInHourBucket10 = false
			break
		}
	}
	if !allInHourBucket10 {
		t.Errorf("Expected all the entries to be in the 10 hour bucket")
	}

	hourlyStatsQuery := `SELECT * FROM hourly_stats`
	hourlyStatsRows, err := db.Query(hourlyStatsQuery)
	if err != nil {
		t.Fatalf("could not query the database for hourly stats: %v", err)
	}
	defer hourlyStatsRows.Close()
	var hourlyStats []HourlyStats
	for hourlyStatsRows.Next() {
		var hourlyStat HourlyStats
		err := hourlyStatsRows.Scan(
			&hourlyStat.Hour,
			&hourlyStat.YearDay,
			&hourlyStat.Year,
			&hourlyStat.Path,
			&hourlyStat.Host,
			&hourlyStat.Pageviews,
			&hourlyStat.UniqueVisitors,
			&hourlyStat.BotViews,
		)
		if err != nil {
			t.Fatalf("unable to parse database hourly stat output, %v", err)
		}
		hourlyStats = append(hourlyStats, hourlyStat)
	}
	if len(hourlyStats) != 3 {
		t.Errorf("Expected 3 entries in hourly stats table, got %d instead", len(hourlyStats))
	}

	if hourlyStats[2].UniqueVisitors != 3 {
		t.Errorf("Expected 3 unique visitors in hourly stats table, got %d instead", hourlyStats[2].UniqueVisitors)
	}

	if hourlyStats[0].Pageviews != 1 {
		t.Errorf("Expected first entry to have 1 page view as count, got %d instead\n", hourlyStats[0].Pageviews)
	}
	if hourlyStats[0].BotViews != 0 {
		t.Errorf("Expected first entry to have 0 bot view as count, got %d instead\n", hourlyStats[0].BotViews)
	}

	if hourlyStats[1].Pageviews != 0 {
		t.Errorf("Expected second entry to have 0 page view as count, got %d instead\n", hourlyStats[1].Pageviews)
	}
	if hourlyStats[1].BotViews != 1 {
		t.Errorf("Expected second entry to have 1 bot view as count, got %d instead\n", hourlyStats[1].BotViews)
	}

	if hourlyStats[2].Pageviews != 1 {
		t.Errorf("Expected third entry to have 1 page view as count, got %d instead\n", hourlyStats[2].Pageviews)
	}
	if hourlyStats[2].BotViews != 0 {
		t.Errorf("Expected third entry to have 0 bot view as count, got %d instead\n", hourlyStats[2].BotViews)
	}

	hourlyStatusCodesQuery := `SELECT * FROM hourly_status_codes`
	hourlyStatusCodesRows, err := db.Query(hourlyStatusCodesQuery)
	if err != nil {
		t.Fatalf("could not query the database for hourly status codes: %v", err)
	}
	defer hourlyStatusCodesRows.Close()
	var hourlyStatusCodes []HourlyStatusCodes
	for hourlyStatusCodesRows.Next() {
		var hourlyStatusCode HourlyStatusCodes
		err := hourlyStatusCodesRows.Scan(
			&hourlyStatusCode.Hour,
			&hourlyStatusCode.YearDay,
			&hourlyStatusCode.Year,
			&hourlyStatusCode.Path,
			&hourlyStatusCode.Host,
			&hourlyStatusCode.StatusCode,
			&hourlyStatusCode.Count,
		)
		if err != nil {
			t.Fatalf("unable to parse database hourly status code output, %v", err)
		}
		hourlyStatusCodes = append(hourlyStatusCodes, hourlyStatusCode)
	}
	if len(hourlyStatusCodes) != 3 {
		t.Errorf("Expected 3 entries in hourly status codes table, got %d instead", len(hourlyStatusCodes))
	}

	all200StatusCodes := true
	for _, hourlyStatusCode := range hourlyStatusCodes {
		if hourlyStatusCode.StatusCode != 200 {
			all200StatusCodes = false
			break
		}
	}
	if !all200StatusCodes {
		t.Errorf("Expected all the entries to have 200 status code")
	}

	hourlyReferrersQuery := `SELECT * FROM hourly_referrers`
	hourlyReferrersRows, err := db.Query(hourlyReferrersQuery)
	if err != nil {
		t.Fatalf("could not query the database for hourly referrers: %v", err)
	}
	defer hourlyReferrersRows.Close()
	var hourlyReferrers []HourlyReferrers
	for hourlyReferrersRows.Next() {
		var hourlyReferrer HourlyReferrers
		err := hourlyReferrersRows.Scan(
			&hourlyReferrer.Hour,
			&hourlyReferrer.YearDay,
			&hourlyReferrer.Year,
			&hourlyReferrer.Path,
			&hourlyReferrer.Host,
			&hourlyReferrer.Referrer,
			&hourlyReferrer.Count,
		)
		if err != nil {
			t.Fatalf("unable to parse database hourly referrers output, %v", err)
		}
		hourlyReferrers = append(hourlyReferrers, hourlyReferrer)
	}
	if len(hourlyReferrers) != 3 {
		t.Errorf("Expected 3 entries in hourly referrers table, got %d instead", len(hourlyReferrers))
	}

	allReferrerCountIsOne := true
	for _, hourlyReferrer := range hourlyReferrers {
		if hourlyReferrer.Count != 1 {
			allReferrerCountIsOne = false
			break
		}
	}
	if !allReferrerCountIsOne {
		t.Errorf("Expected all the entries to have 1 as count")
	}

	ticker := time.NewTicker(1 * time.Second)
	shutdown := make(chan os.Signal, 1)

	wg.Add(1)
	go runPeriodicCleanupsWithWaitGroup(db, ticker, shutdown, &wg)

	shutdown <- syscall.SIGTERM
	close(shutdown)
	wg.Wait()

	var hourly_stats_count int
	err = db.QueryRow("SELECT COUNT(*) FROM hourly_stats").Scan(&hourly_stats_count)
	if err != nil {
		t.Fatalf("Failed to count records: %v", err)
	}

	if hourly_stats_count != 0 {
		t.Errorf("Expected 0 hourly_stats record remaining, got %d", hourly_stats_count)
	}

	var hourly_status_codes_count int
	err = db.QueryRow("SELECT COUNT(*) FROM hourly_status_codes").Scan(&hourly_status_codes_count)
	if err != nil {
		t.Fatalf("Failed to count records: %v", err)
	}

	if hourly_status_codes_count != 0 {
		t.Errorf("Expected 0 hourly_status_codes record remaining, got %d", hourly_status_codes_count)
	}

	var hourly_referrers_count int
	err = db.QueryRow("SELECT COUNT(*) FROM hourly_referrers").Scan(&hourly_referrers_count)
	if err != nil {
		t.Fatalf("Failed to count records: %v", err)
	}

	if hourly_referrers_count != 0 {
		t.Errorf("Expected 0 hourly_referrers record remaining, got %d", hourly_referrers_count)
	}

	var visitor_hashes_count int
	err = db.QueryRow("SELECT COUNT(*) FROM visitor_hashes").Scan(&visitor_hashes_count)
	if err != nil {
		t.Fatalf("Failed to count records: %v", err)
	}

	if visitor_hashes_count != 0 {
		t.Errorf("Expected 0 visitor_hashes record remaining, got %d", visitor_hashes_count)
	}
}

func TestRunDifferentDaysInNearPast(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer closeDB(db)

	logPath := filepath.Join(tempDir, "access.log")

	testLogLines := []string{
		fmt.Sprintf(`192.168.1.1 - - [%s] "GET /index.html HTTP/1.1" 200 1234 "https://google.com" "Mozilla/5.0" "example.com"`, time.Now().AddDate(0, 0, 0).Format("02/Jan/2006:15:04:05 -0700")),
		fmt.Sprintf(`10.0.0.5 - - [%s] "GET /api/data HTTP/1.1" 200 5678 "-" "curl/7.68.0" "example.com"`, time.Now().Add(15*time.Second).AddDate(0, 0, -1).Format("02/Jan/2006:15:04:05 -0700")),
		fmt.Sprintf(`192.168.1.100 - - [%s] "GET /style.css HTTP/1.1" 200 900 "https://example.com" "Mozilla/5.0" "example.com"`, time.Now().Add(15*time.Second).AddDate(0, 0, -2).Format("02/Jan/2006:15:04:05 -0700")),
	}
	createTestLogFile(t, logPath, testLogLines)

	pageViews := make(chan PageView, 100)

	var wg sync.WaitGroup
	wg.Add(1)

	go processPageviewsWithWaitGroup(db, pageViews, &wg)

	tailArgs := []string{"-n", "+1", logPath}
	tailLog(tailArgs, pageViews)
	close(pageViews)

	wg.Wait()

	visitorHashesQuery := `SELECT * FROM visitor_hashes`
	visitorRows, err := db.Query(visitorHashesQuery)
	if err != nil {
		t.Fatalf("could not query the database for vistor hashes: %v", err)
	}
	defer visitorRows.Close()
	var visitorHashes []VisitorHash
	for visitorRows.Next() {
		var visitorHash VisitorHash
		err := visitorRows.Scan(
			&visitorHash.Hash,
			&visitorHash.HourBucket,
			&visitorHash.FirstSeen,
		)
		if err != nil {
			t.Fatalf("unable to parse database visitor hash output, %v", err)
		}
		visitorHashes = append(visitorHashes, visitorHash)
	}

	if len(visitorHashes) != 3 {
		t.Errorf("Expected 3 entries in visitor hash table, got %d instead", len(visitorHashes))
	}

	hourlyStatsQuery := `SELECT * FROM hourly_stats`
	hourlyStatsRows, err := db.Query(hourlyStatsQuery)
	if err != nil {
		t.Fatalf("could not query the database for hourly stats: %v", err)
	}
	defer hourlyStatsRows.Close()
	var hourlyStats []HourlyStats
	for hourlyStatsRows.Next() {
		var hourlyStat HourlyStats
		err := hourlyStatsRows.Scan(
			&hourlyStat.Hour,
			&hourlyStat.YearDay,
			&hourlyStat.Year,
			&hourlyStat.Path,
			&hourlyStat.Host,
			&hourlyStat.Pageviews,
			&hourlyStat.UniqueVisitors,
			&hourlyStat.BotViews,
		)
		if err != nil {
			t.Fatalf("unable to parse database hourly stat output, %v", err)
		}
		hourlyStats = append(hourlyStats, hourlyStat)
	}
	if len(hourlyStats) != 3 {
		t.Errorf("Expected 3 entries in hourly stats table, got %d instead", len(hourlyStats))
	}

	if hourlyStats[2].UniqueVisitors != 3 {
		t.Errorf("Expected 3 unique visitors in hourly stats table, got %d instead", hourlyStats[2].UniqueVisitors)
	}

	if hourlyStats[0].Pageviews != 1 {
		t.Errorf("Expected first entry to have 1 page view as count, got %d instead\n", hourlyStats[0].Pageviews)
	}
	if hourlyStats[0].BotViews != 0 {
		t.Errorf("Expected first entry to have 0 bot view as count, got %d instead\n", hourlyStats[0].BotViews)
	}

	if hourlyStats[1].Pageviews != 0 {
		t.Errorf("Expected second entry to have 0 page view as count, got %d instead\n", hourlyStats[1].Pageviews)
	}
	if hourlyStats[1].BotViews != 1 {
		t.Errorf("Expected second entry to have 1 bot view as count, got %d instead\n", hourlyStats[1].BotViews)
	}

	if hourlyStats[2].Pageviews != 1 {
		t.Errorf("Expected third entry to have 1 page view as count, got %d instead\n", hourlyStats[2].Pageviews)
	}
	if hourlyStats[2].BotViews != 0 {
		t.Errorf("Expected third entry to have 0 bot view as count, got %d instead\n", hourlyStats[2].BotViews)
	}

	hourlyStatusCodesQuery := `SELECT * FROM hourly_status_codes`
	hourlyStatusCodesRows, err := db.Query(hourlyStatusCodesQuery)
	if err != nil {
		t.Fatalf("could not query the database for hourly status codes: %v", err)
	}
	defer hourlyStatusCodesRows.Close()
	var hourlyStatusCodes []HourlyStatusCodes
	for hourlyStatusCodesRows.Next() {
		var hourlyStatusCode HourlyStatusCodes
		err := hourlyStatusCodesRows.Scan(
			&hourlyStatusCode.Hour,
			&hourlyStatusCode.YearDay,
			&hourlyStatusCode.Year,
			&hourlyStatusCode.Path,
			&hourlyStatusCode.Host,
			&hourlyStatusCode.StatusCode,
			&hourlyStatusCode.Count,
		)
		if err != nil {
			t.Fatalf("unable to parse database hourly status code output, %v", err)
		}
		hourlyStatusCodes = append(hourlyStatusCodes, hourlyStatusCode)
	}
	if len(hourlyStatusCodes) != 3 {
		t.Errorf("Expected 3 entries in hourly status codes table, got %d instead", len(hourlyStatusCodes))
	}

	all200StatusCodes := true
	for _, hourlyStatusCode := range hourlyStatusCodes {
		if hourlyStatusCode.StatusCode != 200 {
			all200StatusCodes = false
			break
		}
	}
	if !all200StatusCodes {
		t.Errorf("Expected all the entries to have 200 status code")
	}

	hourlyReferrersQuery := `SELECT * FROM hourly_referrers`
	hourlyReferrersRows, err := db.Query(hourlyReferrersQuery)
	if err != nil {
		t.Fatalf("could not query the database for hourly referrers: %v", err)
	}
	defer hourlyReferrersRows.Close()
	var hourlyReferrers []HourlyReferrers
	for hourlyReferrersRows.Next() {
		var hourlyReferrer HourlyReferrers
		err := hourlyReferrersRows.Scan(
			&hourlyReferrer.Hour,
			&hourlyReferrer.YearDay,
			&hourlyReferrer.Year,
			&hourlyReferrer.Path,
			&hourlyReferrer.Host,
			&hourlyReferrer.Referrer,
			&hourlyReferrer.Count,
		)
		if err != nil {
			t.Fatalf("unable to parse database hourly referrers output, %v", err)
		}
		hourlyReferrers = append(hourlyReferrers, hourlyReferrer)
	}
	if len(hourlyReferrers) != 3 {
		t.Errorf("Expected 3 entries in hourly referrers table, got %d instead", len(hourlyReferrers))
	}

	allReferrerCountIsOne := true
	for _, hourlyReferrer := range hourlyReferrers {
		if hourlyReferrer.Count != 1 {
			allReferrerCountIsOne = false
			break
		}
	}
	if !allReferrerCountIsOne {
		t.Errorf("Expected all the entries to have 1 as count")
	}

	ticker := time.NewTicker(1 * time.Second)
	shutdown := make(chan os.Signal, 1)

	wg.Add(1)
	go runPeriodicCleanupsWithWaitGroup(db, ticker, shutdown, &wg)

	shutdown <- syscall.SIGTERM
	close(shutdown)
	wg.Wait()

	var hourly_stats_count int
	err = db.QueryRow("SELECT COUNT(*) FROM hourly_stats").Scan(&hourly_stats_count)
	if err != nil {
		t.Fatalf("Failed to count records: %v", err)
	}

	if hourly_stats_count != 3 {
		t.Errorf("Expected 3 hourly_stats record remaining, got %d", hourly_stats_count)
	}

	var hourly_status_codes_count int
	err = db.QueryRow("SELECT COUNT(*) FROM hourly_status_codes").Scan(&hourly_status_codes_count)
	if err != nil {
		t.Fatalf("Failed to count records: %v", err)
	}

	if hourly_status_codes_count != 3 {
		t.Errorf("Expected 3 hourly_status_codes record remaining, got %d", hourly_status_codes_count)
	}

	var hourly_referrers_count int
	err = db.QueryRow("SELECT COUNT(*) FROM hourly_referrers").Scan(&hourly_referrers_count)
	if err != nil {
		t.Fatalf("Failed to count records: %v", err)
	}

	if hourly_referrers_count != 3 {
		t.Errorf("Expected 3 hourly_referrers record remaining, got %d", hourly_referrers_count)
	}

	var visitor_hashes_count int
	err = db.QueryRow("SELECT COUNT(*) FROM visitor_hashes").Scan(&visitor_hashes_count)
	if err != nil {
		t.Fatalf("Failed to count records: %v", err)
	}

	if visitor_hashes_count != 2 {
		t.Errorf("Expected 2 visitor_hashes record remaining, got %d", visitor_hashes_count)
	}
}

func TestRunDifferentDaysInDistantPast(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer closeDB(db)

	logPath := filepath.Join(tempDir, "access.log")

	testLogLines := []string{
		fmt.Sprintf(`192.168.1.1 - - [%s] "GET /index.html HTTP/1.1" 200 1234 "https://google.com" "Mozilla/5.0" "example.com"`, time.Now().AddDate(0, 0, -20).Format("02/Jan/2006:15:04:05 -0700")),
		fmt.Sprintf(`10.0.0.5 - - [%s] "GET /api/data HTTP/1.1" 200 5678 "-" "curl/7.68.0" "example.com"`, time.Now().Add(15*time.Second).AddDate(0, 0, -59).Format("02/Jan/2006:15:04:05 -0700")),
		fmt.Sprintf(`192.168.1.100 - - [%s] "GET /style.css HTTP/1.1" 200 900 "https://example.com" "Mozilla/5.0" "example.com"`, time.Now().Add(15*time.Second).AddDate(0, 0, -61).Format("02/Jan/2006:15:04:05 -0700")),
	}
	createTestLogFile(t, logPath, testLogLines)

	pageViews := make(chan PageView, 100)

	var wg sync.WaitGroup
	wg.Add(1)

	go processPageviewsWithWaitGroup(db, pageViews, &wg)

	tailArgs := []string{"-n", "+1", logPath}
	tailLog(tailArgs, pageViews)
	close(pageViews)

	wg.Wait()

	visitorHashesQuery := `SELECT * FROM visitor_hashes`
	visitorRows, err := db.Query(visitorHashesQuery)
	if err != nil {
		t.Fatalf("could not query the database for vistor hashes: %v", err)
	}
	defer visitorRows.Close()
	var visitorHashes []VisitorHash
	for visitorRows.Next() {
		var visitorHash VisitorHash
		err := visitorRows.Scan(
			&visitorHash.Hash,
			&visitorHash.HourBucket,
			&visitorHash.FirstSeen,
		)
		if err != nil {
			t.Fatalf("unable to parse database visitor hash output, %v", err)
		}
		visitorHashes = append(visitorHashes, visitorHash)
	}

	if len(visitorHashes) != 3 {
		t.Errorf("Expected 3 entries in visitor hash table, got %d instead", len(visitorHashes))
	}

	hourlyStatsQuery := `SELECT * FROM hourly_stats`
	hourlyStatsRows, err := db.Query(hourlyStatsQuery)
	if err != nil {
		t.Fatalf("could not query the database for hourly stats: %v", err)
	}
	defer hourlyStatsRows.Close()
	var hourlyStats []HourlyStats
	for hourlyStatsRows.Next() {
		var hourlyStat HourlyStats
		err := hourlyStatsRows.Scan(
			&hourlyStat.Hour,
			&hourlyStat.YearDay,
			&hourlyStat.Year,
			&hourlyStat.Path,
			&hourlyStat.Host,
			&hourlyStat.Pageviews,
			&hourlyStat.UniqueVisitors,
			&hourlyStat.BotViews,
		)
		if err != nil {
			t.Fatalf("unable to parse database hourly stat output, %v", err)
		}
		hourlyStats = append(hourlyStats, hourlyStat)
	}
	if len(hourlyStats) != 3 {
		t.Errorf("Expected 3 entries in hourly stats table, got %d instead", len(hourlyStats))
	}

	if hourlyStats[2].UniqueVisitors != 3 {
		t.Errorf("Expected 3 unique visitors in hourly stats table, got %d instead", hourlyStats[2].UniqueVisitors)
	}

	if hourlyStats[0].Pageviews != 1 {
		t.Errorf("Expected first entry to have 1 page view as count, got %d instead\n", hourlyStats[0].Pageviews)
	}
	if hourlyStats[0].BotViews != 0 {
		t.Errorf("Expected first entry to have 0 bot view as count, got %d instead\n", hourlyStats[0].BotViews)
	}

	if hourlyStats[1].Pageviews != 0 {
		t.Errorf("Expected second entry to have 0 page view as count, got %d instead\n", hourlyStats[1].Pageviews)
	}
	if hourlyStats[1].BotViews != 1 {
		t.Errorf("Expected second entry to have 1 bot view as count, got %d instead\n", hourlyStats[1].BotViews)
	}

	if hourlyStats[2].Pageviews != 1 {
		t.Errorf("Expected third entry to have 1 page view as count, got %d instead\n", hourlyStats[2].Pageviews)
	}
	if hourlyStats[2].BotViews != 0 {
		t.Errorf("Expected third entry to have 0 bot view as count, got %d instead\n", hourlyStats[2].BotViews)
	}

	hourlyStatusCodesQuery := `SELECT * FROM hourly_status_codes`
	hourlyStatusCodesRows, err := db.Query(hourlyStatusCodesQuery)
	if err != nil {
		t.Fatalf("could not query the database for hourly status codes: %v", err)
	}
	defer hourlyStatusCodesRows.Close()
	var hourlyStatusCodes []HourlyStatusCodes
	for hourlyStatusCodesRows.Next() {
		var hourlyStatusCode HourlyStatusCodes
		err := hourlyStatusCodesRows.Scan(
			&hourlyStatusCode.Hour,
			&hourlyStatusCode.YearDay,
			&hourlyStatusCode.Year,
			&hourlyStatusCode.Path,
			&hourlyStatusCode.Host,
			&hourlyStatusCode.StatusCode,
			&hourlyStatusCode.Count,
		)
		if err != nil {
			t.Fatalf("unable to parse database hourly status code output, %v", err)
		}
		hourlyStatusCodes = append(hourlyStatusCodes, hourlyStatusCode)
	}
	if len(hourlyStatusCodes) != 3 {
		t.Errorf("Expected 3 entries in hourly status codes table, got %d instead", len(hourlyStatusCodes))
	}

	all200StatusCodes := true
	for _, hourlyStatusCode := range hourlyStatusCodes {
		if hourlyStatusCode.StatusCode != 200 {
			all200StatusCodes = false
			break
		}
	}
	if !all200StatusCodes {
		t.Errorf("Expected all the entries to have 200 status code")
	}

	hourlyReferrersQuery := `SELECT * FROM hourly_referrers`
	hourlyReferrersRows, err := db.Query(hourlyReferrersQuery)
	if err != nil {
		t.Fatalf("could not query the database for hourly referrers: %v", err)
	}
	defer hourlyReferrersRows.Close()
	var hourlyReferrers []HourlyReferrers
	for hourlyReferrersRows.Next() {
		var hourlyReferrer HourlyReferrers
		err := hourlyReferrersRows.Scan(
			&hourlyReferrer.Hour,
			&hourlyReferrer.YearDay,
			&hourlyReferrer.Year,
			&hourlyReferrer.Path,
			&hourlyReferrer.Host,
			&hourlyReferrer.Referrer,
			&hourlyReferrer.Count,
		)
		if err != nil {
			t.Fatalf("unable to parse database hourly referrers output, %v", err)
		}
		hourlyReferrers = append(hourlyReferrers, hourlyReferrer)
	}
	if len(hourlyReferrers) != 3 {
		t.Errorf("Expected 3 entries in hourly referrers table, got %d instead", len(hourlyReferrers))
	}

	allReferrerCountIsOne := true
	for _, hourlyReferrer := range hourlyReferrers {
		if hourlyReferrer.Count != 1 {
			allReferrerCountIsOne = false
			break
		}
	}
	if !allReferrerCountIsOne {
		t.Errorf("Expected all the entries to have 1 as count")
	}

	ticker := time.NewTicker(1 * time.Second)
	shutdown := make(chan os.Signal, 1)

	wg.Add(1)
	go runPeriodicCleanupsWithWaitGroup(db, ticker, shutdown, &wg)

	shutdown <- syscall.SIGTERM
	close(shutdown)
	wg.Wait()

	var hourly_stats_count int
	err = db.QueryRow("SELECT COUNT(*) FROM hourly_stats").Scan(&hourly_stats_count)
	if err != nil {
		t.Fatalf("Failed to count records: %v", err)
	}

	if hourly_stats_count != 2 {
		t.Errorf("Expected 2 hourly_stats record remaining, got %d", hourly_stats_count)
	}

	var hourly_status_codes_count int
	err = db.QueryRow("SELECT COUNT(*) FROM hourly_status_codes").Scan(&hourly_status_codes_count)
	if err != nil {
		t.Fatalf("Failed to count records: %v", err)
	}

	if hourly_status_codes_count != 2 {
		t.Errorf("Expected 2 hourly_status_codes record remaining, got %d", hourly_status_codes_count)
	}

	var hourly_referrers_count int
	err = db.QueryRow("SELECT COUNT(*) FROM hourly_referrers").Scan(&hourly_referrers_count)
	if err != nil {
		t.Fatalf("Failed to count records: %v", err)
	}

	if hourly_referrers_count != 2 {
		t.Errorf("Expected 2 hourly_referrers record remaining, got %d", hourly_referrers_count)
	}

	var visitor_hashes_count int
	err = db.QueryRow("SELECT COUNT(*) FROM visitor_hashes").Scan(&visitor_hashes_count)
	if err != nil {
		t.Fatalf("Failed to count records: %v", err)
	}

	if visitor_hashes_count != 0 {
		t.Errorf("Expected 0 visitor_hashes record remaining, got %d", visitor_hashes_count)
	}
}

func newTestDB(dbPath string) (*sql.DB, error) {
	dir := filepath.Dir(dbPath)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return nil, fmt.Errorf("could not create test db directory: %w", err)
	}
	return openDB(dbPath)
}

func setupTestDB(t *testing.T) (*sql.DB, string) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	db, err := newTestDB(dbPath)
	if err != nil {
		t.Fatalf("Unable to create test database: %v", err)
	}
	return db, tempDir
}

func processPageviewsWithWaitGroup(db *sql.DB, pageViews <-chan PageView, wg *sync.WaitGroup) {
	defer wg.Done()
	processPageviews(db, pageViews)
}

func runPeriodicCleanupsWithWaitGroup(db *sql.DB, ticker *time.Ticker, shutdown <-chan os.Signal, wg *sync.WaitGroup) {
	defer wg.Done()
	runPeriodicCleanup(db, ticker, shutdown)
}

func createTestLogFile(t *testing.T, logPath string, logLines []string) {
	dir := filepath.Dir(logPath)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		t.Fatalf("Failed to create log directory: %v", err)
	}

	file, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("Failed to create log file: %v", err)
	}
	defer file.Close()

	for _, line := range logLines {
		_, err := file.WriteString(line + "\n")
		if err != nil {
			t.Fatalf("Failed to write log line: %v", err)
		}
	}
}
