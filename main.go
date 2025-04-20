package main

import (
	"encoding/csv"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/patrickmn/go-cache"
)

func main() {
	tmpl := template.Must(template.ParseFiles("pages/index.html", "pages/result.html"))

	c := cache.New(72*time.Hour, 1*time.Hour)

	fs := http.FileServer(http.Dir("assets"))

	http.Handle("/assets/", http.StripPrefix("/assets/", fs))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		tmpl.ExecuteTemplate(w, "index.html", nil)
	})

	http.HandleFunc("/analyze", func(w http.ResponseWriter, r *http.Request) {

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		err := r.ParseMultipartForm(10 << 20)
		if err != nil {
			http.Error(w, "Failed to parse form: "+err.Error(), http.StatusBadRequest)
			return
		}

		file, fileHeader, err := r.FormFile("csvFile")
		if err != nil {
			http.Error(w, "Failed to get file: "+err.Error(), http.StatusBadRequest)
			return
		}

		defer file.Close()

		if !strings.HasSuffix(fileHeader.Filename, ".csv") {
			http.Error(w, "Only CSV files are allowed", http.StatusBadRequest)
			return
		}

		reader := csv.NewReader(file)

		diffs := make([]float64, 0)

		firstLine := true

		for {
			record, err := reader.Read()

			if err == io.EOF {
				break
			}

			if firstLine {
				firstLine = false
				continue
			}

			userRating, _ := strconv.ParseFloat(record[4], 64)

			if x, found := c.Get(record[3]); found {

				rating, _ := x.(float64)

				diffs = append(diffs, rating-userRating)
				continue
			}

			resp, err := http.Get(record[3])

			if err != nil {
				log.Fatal("Failed to fetch URL:", err)
			}

			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)

			if err != nil {
				log.Fatal("Failed to read response body:", err)
			}

			re := regexp.MustCompile(`"ratingValue"\s*:\s*([0-9.]+)`)
			matches := re.FindSubmatch(body)

			rating, _ := strconv.ParseFloat(string(matches[1]), 64)

			c.Set(record[3], rating, cache.DefaultExpiration)

			diffs = append(diffs, rating-userRating)
		}

		sum := 0.0

		for _, d := range diffs {
			if d < 0 {
				d = -d
			}

			sum = sum + d
		}

		avg := sum / float64(len(diffs))

		ranking := ""

		if avg > 2 {
			ranking = "Very Contrarian"
		} else if avg > 1 {
			ranking = "Contrarian"
		} else if avg > 0.5 {
			ranking = "A bit of a Contrarian"
		} else {
			ranking = "No personal opinion"
		}

		result := struct {
			Score   string
			Ranking string
		}{
			Score:   fmt.Sprintf("%.2f", avg),
			Ranking: ranking,
		}

		tmpl.ExecuteTemplate(w, "result.html", result)
	})

	http.ListenAndServe(":8080", nil)
}
