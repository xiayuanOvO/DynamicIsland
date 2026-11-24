package island

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"strconv"
	"strings"
	"sync"
)

//go:embed china_cities.tsv.gz
var chinaCitiesGZ []byte

const chinaCityIDBase = 1_000_000_000

var (
	chinaCitiesOnce sync.Once
	chinaCitiesList []weatherCity
)

func loadChinaCities() []weatherCity {
	chinaCitiesOnce.Do(func() {
		reader, err := gzip.NewReader(bytes.NewReader(chinaCitiesGZ))
		if err != nil {
			return
		}
		defer reader.Close()
		scanner := bufio.NewScanner(reader)
		id := chinaCityIDBase
		for scanner.Scan() {
			parts := strings.Split(scanner.Text(), "\t")
			if len(parts) != 4 {
				continue
			}
			lat, err1 := strconv.ParseFloat(parts[2], 64)
			lon, err2 := strconv.ParseFloat(parts[3], 64)
			if err1 != nil || err2 != nil {
				continue
			}
			id++
			chinaCitiesList = append(chinaCitiesList, weatherCity{
				ID:        id,
				Name:      parts[0],
				Region:    parts[1],
				Latitude:  lat,
				Longitude: lon,
			})
		}
	})
	return chinaCitiesList
}

func searchChinaCities(query string, limit int) []weatherCity {
	terms := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	if len(terms) == 0 || limit <= 0 {
		return nil
	}
	cities := loadChinaCities()
	var exact, prefix, contains, regionMatch []weatherCity
	single := len(terms) == 1
	q := terms[0]
	for _, city := range cities {
		nameLower := strings.ToLower(city.Name)
		fullLower := nameLower + " " + strings.ToLower(city.Region)
		matched := true
		for _, term := range terms {
			if !strings.Contains(fullLower, term) {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		switch {
		case single && nameLower == q:
			exact = append(exact, city)
		case single && strings.HasPrefix(nameLower, q):
			prefix = append(prefix, city)
		case single && strings.Contains(nameLower, q):
			contains = append(contains, city)
		default:
			regionMatch = append(regionMatch, city)
		}
	}
	results := make([]weatherCity, 0, min(limit, len(exact)+len(prefix)+len(contains)+len(regionMatch)))
	for _, group := range [][]weatherCity{exact, prefix, contains, regionMatch} {
		for _, city := range group {
			results = append(results, city)
			if len(results) == limit {
				return results
			}
		}
	}
	return results
}
