package main

import (
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	_ "golang.org/x/net/publicsuffix"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

func init() {
	if os.Getenv("EH_USERNAME") == "" || os.Getenv("EH_PASSWORD") == "" {
		panic("Missing EH_USERNAME or EH_PASSWORD environment variables")
	}

}

var EH_USERNAME = os.Getenv("EH_USERNAME")
var EH_PASSWORD = os.Getenv("EH_PASSWORD")

var (
	regionTraffic = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hentaiathome_region_traffic",
		Help: "The total amount of traffic in MB/s for each region",
	}, []string{"region"})
	regionHitsPerSecond = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hentaiathome_region_hits_per_second",
		Help: "The total amount of hits per second for each region",
	}, []string{"region"})
	regionQuality = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hentaiathome_region_quality",
		Help: "The total amount of quality for each region",
	}, []string{"region"})
	regionCoverage = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hentaiathome_region_coverage",
		Help: "The total amount of coverage for each region",
	}, []string{"region"})

	clientHitrate = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hentaiathome_client_hitrate",
		Help: "The total amount of hits per minute for each client",
	}, []string{"client"})
	clientHathrate = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hentaiathome_client_hathrate",
		Help: "The hath generated per day for each client",
	}, []string{"client"})
	clientQuality = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hentaiathome_client_quality",
		Help: "The total amount of quality for each client",
	}, []string{"client"})
	clientTrust = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hentaiathome_client_trust",
		Help: "The total amount of trust for each client",
	}, []string{"client"})
	clientFilesServed = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hentaiathome_client_files_served",
		Help: "The total amount of files served for each client",
	}, []string{"client"})
	clientOnlineState = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hentaiathome_client_online_state",
		Help: "The current online state for each client",
	}, []string{"client"})
)

var globalHttpClient = &http.Client{
	Jar: nil,
}

func main() {
	globalHttpClient.Jar, _ = cookiejar.New(nil)
	globalHttpClient.Jar.SetCookies(
		&url.URL{Host: "e-hentai.org"},
		[]*http.Cookie{
			{
				Name:  "ipb_coppa",
				Value: "0",
			},
		},
	)

	// Data is refreshed every 60 seconds
	go func() {
		for {
			log.Println("Fetching data from e-hentai.org")
			fetchEHentai()
			time.Sleep(60 * time.Second)
		}
	}()

	// Start the HTTP server
	http.Handle("/metrics", promhttp.Handler())
	err := http.ListenAndServe(":2112", nil)

	if err != nil {
		panic(err)
	}
}

func fetchEHentai() {
	req, err := http.NewRequest("GET", "https://e-hentai.org/hentaiathome.php", nil)
	if err != nil {
		panic(err)
	}

	req.Header.Set("User-Agent", "Hentai@Home Prometheus Exporter")
	req.Header.Set("Referer", "https://e-hentai.org/hentaiathome.php")

	res, err := globalHttpClient.Do(req)

	if strings.Contains(res.Request.URL.String(), "bounce_login.php") {
		log.Info("Not logged in, logging in...")
		// Login
		loginEHentai()
		fetchEHentai()
	} else {
		// Extract with goquery
		var responseBody string
		res.Body.Read([]byte(responseBody))

		log.Debug("Response length: ", len(responseBody))

		// Parse the response
		doc, err := goquery.NewDocumentFromReader(res.Body)
		if err != nil {
			panic(err)
		}

		// Extract the region traffic
		doc.Find("#hathstats tr").Each(func(i int, s *goquery.Selection) {
			if i != 0 { // Skip the header row
				hathRegion := s.Find("td").Eq(0).Text()
				networkLoad := s.Find("td").Eq(3).Text()
				hitsPerSec := s.Find("td").Eq(4).Text()
				coverage := s.Find("td").Eq(5).Text()
				hitsPerGB := s.Find("td").Eq(6).Text()
				quality := s.Find("td").Eq(7).Text()

				fmt.Printf("Region: %s, Network Load: %s, Hits/sec: %s, Coverage: %s, Hits/GB: %s, Quality: %s\n",
					hathRegion, networkLoad, hitsPerSec, coverage, hitsPerGB, quality)

				traffic, err := strconv.ParseFloat(strings.TrimSuffix(networkLoad, " MB/s"), 64)
				if err != nil {
					log.Println("Failed to parse traffic for ", hathRegion, ": ", err)
					traffic = 0
				}

				regionTraffic.With(prometheus.Labels{"region": hathRegion}).Set(traffic)

				hitsPerSecFloat, err := strconv.ParseFloat(hitsPerSec, 64)
				if err != nil {
					hitsPerSecFloat = 0
				}
				regionHitsPerSecond.With(prometheus.Labels{"region": hathRegion}).Set(hitsPerSecFloat)

				coverageFloat, err := strconv.ParseFloat(coverage, 64)
				if err != nil {
					coverageFloat = 0
				}
				regionCoverage.With(prometheus.Labels{"region": hathRegion}).Set(coverageFloat)

				qualityFloat, err := strconv.ParseFloat(quality, 64)
				if err != nil {
					qualityFloat = 0
				}
				regionQuality.With(prometheus.Labels{"region": hathRegion}).Set(qualityFloat)

				log.Println("Set region metrics for ", hathRegion)
			}
		})

		// Extract the client stats
		doc.Find("#hct tr").Each(func(i int, s *goquery.Selection) {
			if i != 0 { // Skip the header row
				client := s.Find("td").Eq(0).Text()
				id := s.Find("td").Eq(1).Text()
				status := s.Find("td").Eq(2).Text()
				created := s.Find("td").Eq(3).Text()
				lastSeen := s.Find("td").Eq(4).Text()
				filesServed := s.Find("td").Eq(5).Text()
				clientIP := s.Find("td").Eq(6).Text()
				port := s.Find("td").Eq(7).Text()
				version := s.Find("td").Eq(8).Text()
				maxSpeed := s.Find("td").Eq(9).Text()
				trust := s.Find("td").Eq(10).Text()
				quality := s.Find("td").Eq(11).Text()
				hitrate := s.Find("td").Eq(12).Text()
				hathrate := s.Find("td").Eq(13).Text()
				country := s.Find("td").Eq(14).Text()

				log.Infof("Client: %s, ID: %s, Status: %s, Created: %s, Last Seen: %s, Files Served: %s, Client IP: %s, Port: %s, Version: %s, Max Speed: %s, Trust: %s, Quality: %s, Hitrate: %s, Hathrate: %s, Country: %s\n",
					client, id, status, created, lastSeen, filesServed, clientIP, port, version, maxSpeed, trust, quality, hitrate, hathrate, country)

				hitrateFloat, err := strconv.ParseFloat(strings.TrimSuffix(hitrate, " / min"), 64)
				if err != nil {
					log.Error("Failed to parse hitrate for ", client, ": ", err)
					hitrateFloat = 0
				}
				clientHitrate.With(prometheus.Labels{"client": client}).Set(hitrateFloat)

				hathrateFloat, err := strconv.ParseFloat(strings.TrimSuffix(hathrate, " / day"), 64)
				if err != nil {
					log.Error("Failed to parse hathrate for ", client, ": ", err)
					hathrateFloat = 0
				}
				clientHathrate.With(prometheus.Labels{"client": client}).Set(hathrateFloat)

				qualityFloat, err := strconv.ParseFloat(quality, 64)
				if err != nil {
					log.Error("Failed to parse quality for ", client, ": ", err)
					qualityFloat = 0
				}
				clientQuality.With(prometheus.Labels{"client": client}).Set(qualityFloat)

				trustFloat, err := strconv.ParseFloat(trust, 64)
				if err != nil {
					log.Error("Failed to parse trust for ", client, ": ", err)
					trustFloat = 0
				}
				clientTrust.With(prometheus.Labels{"client": client}).Set(trustFloat)

				filesServedFloat, err := strconv.ParseFloat(strings.Replace(filesServed, ",", "", -1), 64)
				if err != nil {
					log.Error("Failed to parse files served for ", client, ": ", err)
					filesServedFloat = 0
				}
				clientFilesServed.With(prometheus.Labels{"client": client}).Set(filesServedFloat)

				if status == "Online" {
					clientOnlineState.With(prometheus.Labels{"client": client}).Set(1)
				} else {
					clientOnlineState.With(prometheus.Labels{"client": client}).Set(0)
				}

				log.Infof("Set client metrics for", client)
			}
		})

		log.Println("Updated metrics")
	}

}

func loginEHentai() bool {
	form, err := globalHttpClient.PostForm("https://forums.e-hentai.org/index.php?act=Login&CODE=01", url.Values{
		"referer":          {"https://e-hentai.org"},
		"UserName":         {os.Getenv("EH_USERNAME")},
		"PassWord":         {os.Getenv("EH_PASSWORD")},
		"CookieDate":       {"1"},
		"b":                {"d"},
		"bt":               {"1-11"},
		"ipb_login_submit": {"Login!"},
	})
	logrus.Debug("Sent login request")
	if err != nil {
		panic(err)
	}

	// Check if the login was successful
	cookie := form.Header.Get("Set-Cookie")
	if !strings.Contains(cookie, "ipb_session_id") {
		logrus.Error("Login failed")
		logrus.Error(form)

		panic("Login failed")
	}

	log.Debug("Login successful, finding redirect URL...")

	var body, _ = io.ReadAll(form.Body)

	// Get the redirect URL
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	redirectLink := doc.Find("a").Eq(0).AttrOr("href", "")
	log.Debug("Redirect link: ", redirectLink)

	// Follow the redirect to get more cookies
	_, err = globalHttpClient.Get(redirectLink)
	if err != nil {
		panic(err)
	}

	log.Debug("Redirected to ", redirectLink)
	log.Println("Login successful")

	return true
}
