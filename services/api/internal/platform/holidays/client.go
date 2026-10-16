package holidays

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const Source = "https://indonesian-holidays.ahrulsyamil.com"

var ErrUnavailable = errors.New("holiday data unavailable")

type Day struct {
	Date string `json:"date"`
	Name string `json:"name"`
	Type string `json:"type"`
}
type Client struct {
	base string
	http http.Client
}

func New(base string) *Client {
	return &Client{base: strings.TrimRight(base, "/"), http: http.Client{Timeout: 4 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (c *Client) Year(ctx context.Context, year int) ([]Day, error) {
	if year < 1900 || year > 2200 {
		return nil, ErrUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/holidays/"+strconv.Itoa(year), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	response, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (512<<10)+1))
	if err != nil || len(body) > 512<<10 {
		return nil, ErrUnavailable
	}
	var data struct {
		Meta struct{ Year, Count int }
		Data []Day
	}
	if json.Unmarshal(body, &data) != nil || data.Meta.Year != year || data.Meta.Count != len(data.Data) || len(data.Data) == 0 || len(data.Data) > 400 {
		return nil, ErrUnavailable
	}
	seen := map[string]bool{}
	for _, day := range data.Data {
		date, err := time.Parse("2006-01-02", day.Date)
		key := fmt.Sprint(day.Date, "/", day.Type, "/", day.Name)
		if err != nil || date.Year() != year || strings.TrimSpace(day.Name) != day.Name || day.Name == "" || utf8.RuneCountInString(day.Name) > 200 || (day.Type != "national_holiday" && day.Type != "joint_leave") || seen[key] {
			return nil, ErrUnavailable
		}
		seen[key] = true
	}
	return data.Data, nil
}
