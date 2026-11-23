package exchangerate

import (
	"os"
	"testing"
	"time"
)

func TestSMBCOfficialPDF(t *testing.T) {
	for _, date := range []string{"20261015", "20261013"} {
		t.Run(date, func(t *testing.T) {
			body, err := os.ReadFile("testdata/smbc-" + date + ".pdf")
			if err != nil {
				t.Fatal(err)
			}
			d, _ := time.Parse("20060102", date)
			rates, err := ParsePDF(body, d.Format("2006-01-02"))
			if err != nil {
				t.Fatal(err)
			}
			if len(rates) != 3 {
				t.Fatal(rates)
			}
			if date == "20261015" {
				for code, want := range map[string]int64{"USD": 15799000000, "EUR": 17687000000, "HKD": 2013000000} {
					if rates[code] != want {
						t.Errorf("%s got %d want %d", code, rates[code], want)
					}
				}
			}
			if _, err := ParsePDF(body, "2026-01-01"); err == nil {
				t.Fatal("mismatched date accepted")
			}
		})
	}
	if _, err := ParsePDF([]byte("<html>Error</html>"), "2026-10-15"); err == nil {
		t.Fatal("invalid document accepted")
	}
}

func TestLatestSMBCPublication(t *testing.T) {
	index := []byte(`<a href="/market/backnumber/fixing/pdf_daily/recent/fixing2026093020260930.pdf">old</a><a href="/market/backnumber/fixing/pdf_daily/now/fixing2026101520261015.pdf">today</a><a href="/market/backnumber/fixing/pdf_daily/now/fixing2026101920261019.pdf">future</a>`)
	for _, date := range []string{"2026-10-15T08:00:00+09:00", "2026-10-17T12:00:00+09:00"} {
		now, _ := time.Parse(time.RFC3339, date)
		p, err := LatestPublication(index, now)
		if err != nil || p.Date != "2026-10-15" {
			t.Fatalf("%+v %v", p, err)
		}
	}
	if _, err := LatestPublication([]byte("changed site"), time.Now()); err == nil {
		t.Fatal("missing link accepted")
	}
}

func TestMidpointAndSchedule(t *testing.T) {
	rate, err := Midpoint("20.56", "19.70")
	if err != nil || rate != 2013000000 {
		t.Fatal(rate, err)
	}
	for _, pair := range [][2]string{{"1", "2"}, {"3", "0"}, {"x", "3"}} {
		if _, err := Midpoint(pair[0], pair[1]); err == nil {
			t.Fatal(pair)
		}
	}
	for _, v := range []struct {
		date string
		want bool
	}{{"2026-10-15T09:00:00+09:00", true}, {"2026-10-15T19:59:00+09:00", true}, {"2026-10-15T20:00:00+09:00", false}, {"2026-10-17T12:00:00+09:00", false}} {
		now, _ := time.Parse(time.RFC3339, v.date)
		if Scheduled(now) != v.want {
			t.Fatal(v)
		}
	}
}
