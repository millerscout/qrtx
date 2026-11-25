package sdd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type planScenario struct {
	ID                string   `json:"id"`
	Request           string   `json:"request"`
	AcceptanceBullets int      `json:"acceptance_bullets"`
	Overlap           bool     `json:"overlap"`
	Hit               string   `json:"hit"`
	Paths             []string `json:"paths,omitempty"`
	ExpectBand        string   `json:"expect_band"`
	ExpectDomain      string   `json:"expect_domain"`
}

type planBaseline struct {
	ID     string `json:"id"`
	Band   string `json:"band"`
	Domain string `json:"domain"`
}

func loadPlanJSONL[T any](path string) ([]T, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []T
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var row T
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, sc.Err()
}

func benchStatus(baseBand, gotBand, expectBand, baseDomain, gotDomain string) string {
	if gotBand != expectBand {
		return "regressed"
	}
	if strings.TrimSpace(baseBand) == "" && strings.TrimSpace(baseDomain) == "" {
		return "improved"
	}
	domainSame := strings.TrimSpace(baseDomain) == "" || baseDomain == gotDomain
	if baseBand == gotBand && domainSame {
		return "unchanged"
	}
	return "regressed"
}

func applyBenchStatus(domain string, paths []string) string {
	if len(paths) == 0 {
		return "apply_skipped"
	}
	for _, p := range paths {
		if !inDomain(domain, p) {
			return "apply_fail"
		}
	}
	return "apply_ok"
}

func formatBenchCounts(improved, regressed, unchanged, applyOK, applyFail, applySkipped int) string {
	return fmt.Sprintf("improved %d\nregressed %d\nunchanged %d\napply_ok %d\napply_fail %d\napply_skipped %d", improved, regressed, unchanged, applyOK, applyFail, applySkipped)
}
