package calendar

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

var (
	cache   = make(map[int][]bool) // year -> day_index -> is_non_working
	cacheMu sync.RWMutex
	client  = &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
)

func fetchYear(year int) ([]bool, error) {
	url := fmt.Sprintf("https://isdayoff.ru/api/getdata?year=%d", year)
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("isdayoff request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("isdayoff status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("isdayoff read: %w", err)
	}

	data := string(body)
	days := make([]bool, len(data))
	for i, ch := range data {
		days[i] = ch == '1'
	}
	return days, nil
}

func getYear(year int) []bool {
	cacheMu.RLock()
	if yearData, ok := cache[year]; ok {
		cacheMu.RUnlock()
		return yearData
	}
	cacheMu.RUnlock()

	yearData, err := fetchYear(year)
	if err != nil {
		// Fallback: not cached, return nil — callers will use weekend fallback
		return nil
	}

	cacheMu.Lock()
	cache[year] = yearData
	cacheMu.Unlock()
	return yearData
}

// IsNonWorking returns true if the given date is a non-working day
// according to the Russian production calendar.
// Falls back to simple weekend check if API is unavailable.
func IsNonWorking(t time.Time) bool {
	yearData := getYear(t.Year())
	dayIdx := t.YearDay() - 1
	if yearData != nil && dayIdx >= 0 && dayIdx < len(yearData) {
		return yearData[dayIdx]
	}
	// Fallback
	wd := t.Weekday()
	return wd == time.Saturday || wd == time.Sunday
}

// IsWorkDay returns true if the given date is a working day.
func IsWorkDay(t time.Time) bool {
	return !IsNonWorking(t)
}

// SkipToWorkDay advances to the next working day if the given date is non-working.
func SkipToWorkDay(t time.Time) time.Time {
	for IsNonWorking(t) {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

// AddWorkDays adds n working days to a date (0 = same day).
func AddWorkDays(from time.Time, n int) time.Time {
	t := from
	for i := 0; i < n; i++ {
		t = t.AddDate(0, 0, 1)
		for IsNonWorking(t) {
			t = t.AddDate(0, 0, 1)
		}
	}
	return t
}

// SubtractWorkDays subtracts n working days from a date.
func SubtractWorkDays(from time.Time, n int) time.Time {
	t := from
	for n > 0 {
		t = t.AddDate(0, 0, -1)
		if IsWorkDay(t) {
			n--
		}
	}
	return t
}

// GetNonWorkingDays returns all non-working dates in [from, to] as YYYY-MM-DD strings.
func GetNonWorkingDays(from, to time.Time) []string {
	var result []string
	d := from
	for !d.After(to) {
		if IsNonWorking(d) {
			result = append(result, d.Format("2006-01-02"))
		}
		d = d.AddDate(0, 0, 1)
	}
	return result
}
