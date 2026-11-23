// Package exchangerate reads SMBC's daily published exchange quotations.
// It deliberately does not use the bank's intraday retail deposit rates.
package exchangerate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ledongthuc/pdf"
)

const Provider = "SMBC TTM"
const IndexURL = "https://www2.smbc.co.jp/market/backnumber/fixing/index.html"
const Scale int64 = 100000000

var JST = time.FixedZone("JST", 9*60*60)
var Currencies = []string{"USD", "HKD", "EUR"}

type Publication struct {
	Date      string           `json:"date"`
	SourceURL string           `json:"sourceUrl"`
	SHA256    string           `json:"sha256"`
	Rates     map[string]int64 `json:"rates"`
	Unchanged bool             `json:"unchanged"`
}

type Fetcher struct{ Client *http.Client }

func NewFetcher() *Fetcher {
	return &Fetcher{Client: &http.Client{Timeout: 25 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 3 || !officialURL(r.URL) {
			return errors.New("SMBC redirected outside its official site")
		}
		return nil
	}}}
}

func officialURL(u *url.URL) bool {
	return u.Scheme == "https" && (u.Host == "www2.smbc.co.jp" || u.Host == "www.smbc.co.jp") && u.User == nil
}

func (f *Fetcher) download(ctx context.Context, address string, limit int64) ([]byte, error) {
	u, err := url.Parse(address)
	if err != nil || !officialURL(u) {
		return nil, errors.New("invalid SMBC source URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ZaikoKanri/1.0 (daily SMBC TTM lookup)")
	req.Header.Set("Cache-Control", "no-cache")
	res, err := f.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("SMBC request failed: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("SMBC returned HTTP %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errors.New("SMBC document exceeds size limit")
	}
	return body, nil
}

var linkPattern = regexp.MustCompile(`(?i)href\s*=\s*["'](/market/backnumber/fixing/pdf_daily/(?:now|recent)/fixing(\d{8})\d{8}\.pdf)["']`)

func LatestPublication(index []byte, now time.Time) (Publication, error) {
	var candidates []Publication
	today := now.In(JST).Format("20060102")
	for _, m := range linkPattern.FindAllSubmatch(index, -1) {
		day, err := time.ParseInLocation("20060102", string(m[2]), JST)
		if err != nil || string(m[2]) > today {
			continue
		}
		candidates = append(candidates, Publication{Date: day.Format("2006-01-02"), SourceURL: "https://www2.smbc.co.jp" + string(m[1])})
	}
	if len(candidates) == 0 {
		return Publication{}, errors.New("SMBC daily publication link not found")
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Date > candidates[j].Date })
	return candidates[0], nil
}

func (f *Fetcher) Fetch(ctx context.Context, now time.Time, knownDate string) (Publication, error) {
	index, err := f.download(ctx, IndexURL, 2<<20)
	if err != nil {
		return Publication{}, err
	}
	p, err := LatestPublication(index, now)
	if err != nil {
		return p, err
	}
	day, _ := time.ParseInLocation("2006-01-02", p.Date, JST)
	// Do not silently accept an indefinitely stale source after a site change.
	if now.Sub(day) > 10*24*time.Hour {
		return p, errors.New("SMBC publication is more than 10 days old")
	}
	if knownDate != "" && p.Date < knownDate {
		return p, errors.New("SMBC publication date moved backwards")
	}
	if p.Date == knownDate {
		p.Unchanged = true
		return p, nil
	}
	body, err := f.download(ctx, p.SourceURL, 4<<20)
	if err != nil {
		return p, err
	}
	p.Rates, err = ParsePDF(body, p.Date)
	if err != nil {
		return p, err
	}
	hash := sha256.Sum256(body)
	p.SHA256 = hex.EncodeToString(hash[:])
	return p, nil
}

type word struct {
	X, Y float64
	Text string
}

func ParsePDF(data []byte, expectedDate string) (rates map[string]int64, err error) {
	defer func() {
		if recover() != nil {
			rates = nil
			err = errors.New("SMBC PDF could not be parsed")
		}
	}()
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return nil, errors.New("SMBC response is not a PDF")
	}
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	if r.NumPage() != 1 {
		return nil, errors.New("unexpected SMBC PDF page count")
	}
	rows, err := r.Page(1).GetTextByRow()
	if err != nil {
		return nil, err
	}
	var words []word
	for _, row := range rows {
		for _, w := range row.Content {
			words = append(words, word{w.X, w.Y, strings.TrimSpace(w.S)})
		}
	}
	return parseWords(words, expectedDate)
}

var datePattern = regexp.MustCompile(`Date:\s*([A-Za-z]{3})\.?\s*(\d{1,2}),\s*(\d{4})`)
var numberPattern = regexp.MustCompile(`^\d+(?:\.\d+)?$`)

func parseWords(words []word, expectedDate string) (map[string]int64, error) {
	var all strings.Builder
	var tts, ttb *word
	for i := range words {
		w := &words[i]
		all.WriteString(w.Text)
		all.WriteByte(' ')
		if w.Text == "T.T.S" {
			tts = w
		}
		if w.Text == "T.T.B" {
			ttb = w
		}
	}
	m := datePattern.FindStringSubmatch(all.String())
	if len(m) != 4 {
		return nil, errors.New("SMBC PDF date not found")
	}
	day, err := time.Parse("Jan 2 2006", m[1]+" "+m[2]+" "+m[3])
	if err != nil || day.Format("2006-01-02") != expectedDate {
		return nil, errors.New("SMBC PDF date does not match publication link")
	}
	if tts == nil || ttb == nil || tts.X >= ttb.X || math.Abs(tts.Y-ttb.Y) > 3 {
		return nil, errors.New("SMBC TTS/TTB columns not found")
	}
	rates := map[string]int64{}
	for _, currency := range Currencies {
		var label *word
		for i := range words {
			w := &words[i]
			// The USANCE section repeats USD/EUR below the quotation table.
			if w.Text == currency && w.X < tts.X-50 && w.Y < tts.Y-5 && w.Y > tts.Y-220 {
				if label != nil {
					return nil, fmt.Errorf("duplicate %s quotation row", currency)
				}
				label = w
			}
		}
		if label == nil {
			return nil, fmt.Errorf("%s quotation row not found", currency)
		}
		cell := func(left, right float64) (string, error) {
			value := ""
			for _, w := range words {
				if math.Abs(w.Y-label.Y) <= 3 && w.X >= left && w.X < right {
					if value != "" {
						return "", errors.New("ambiguous SMBC quotation cell")
					}
					value = w.Text
				}
			}
			if !numberPattern.MatchString(value) {
				return "", errors.New("missing or invalid SMBC quotation cell")
			}
			return value, nil
		}
		unit, e := cell(label.X+15, label.X+45)
		if e != nil || unit != "1" {
			return nil, fmt.Errorf("invalid %s quotation unit", currency)
		}
		s, e := cell(tts.X-12, tts.X+35)
		if e != nil {
			return nil, fmt.Errorf("%s TTS: %w", currency, e)
		}
		b, e := cell(ttb.X-12, ttb.X+35)
		if e != nil {
			return nil, fmt.Errorf("%s TTB: %w", currency, e)
		}
		rate, e := Midpoint(s, b)
		if e != nil {
			return nil, e
		}
		rates[currency] = rate
	}
	return rates, nil
}

// Midpoint calculates TTM from the same daily publication's TTS and TTB,
// using decimal arithmetic rather than binary floating-point rounding.
func Midpoint(tts, ttb string) (int64, error) {
	s, ok := new(big.Rat).SetString(tts)
	if !ok {
		return 0, errors.New("invalid TTS")
	}
	b, ok := new(big.Rat).SetString(ttb)
	if !ok || b.Sign() <= 0 || s.Cmp(b) < 0 {
		return 0, errors.New("invalid TTS/TTB spread")
	}
	v := new(big.Rat).Mul(new(big.Rat).Add(s, b), big.NewRat(Scale, 2))
	if !v.IsInt() || !v.Num().IsInt64() {
		return 0, errors.New("invalid TTM precision")
	}
	n := v.Num().Int64()
	if n < Scale/100 || n > 10000*Scale {
		return 0, errors.New("TTM outside supported range")
	}
	return n, nil
}

func Scheduled(now time.Time) bool {
	t := now.In(JST)
	return t.Weekday() != time.Saturday && t.Weekday() != time.Sunday && t.Hour() >= 9 && t.Hour() < 20
}
