package main

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/yosuke-furukawa/json5/encoding/json5"
	"gopkg.in/urfave/cli.v1"
)

func main() {
	app := cli.NewApp()
	app.Name = "bandcamp-download"
	app.Author = "Vadim Chernov"
	app.Email = "dimuls@yandex.ru"
	app.Version = "0.0.1"

	commonFlags := []cli.Flag{
		cli.StringFlag{
			Name:  "url, u",
			Usage: "album page URL",
		},
		cli.StringFlag{
			Name:  "path, p",
			Usage: "path where to download",
			Value: "./",
		},
		cli.BoolFlag{
			Name:  "verbose, v",
			Usage: "show debug messages",
		},
	}

	app.Commands = []cli.Command{
		{
			Name:    "album",
			Aliases: []string{"a"},
			Usage:   "download album from album page",
			Action:  downloadAlbumAction,
			Flags:   commonFlags,
		},
		{
			Name:    "albums",
			Aliases: []string{"as"},
			Usage:   "download albums from albums page",
			Action:  downloadAlbumsAction,
			Flags:   commonFlags,
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to parse arguments")
	}
}

func extractAlbumJSON(body string) (string, error) {
	const (
		startWith = "var TralbumData = {"
		stopWith  = "};"
	)

	startIndex := strings.Index(body, startWith)

	if startIndex == -1 {
		return "", errors.New("unable to find album data")
	}

	temp := body[startIndex+len(startWith)-1:]

	endIndex := strings.Index(temp, stopWith)

	if endIndex == -1 {
		return "", errors.New("unable to find album data end")
	}

	albumJSON := temp[:endIndex+1]

	albumJSON = fixAlbumJSON(albumJSON)

	return albumJSON, nil
}

func fixAlbumJSON(albumJSON string) string {
	// We are fixing URLs like:
	// 		"http://verbalclick.bandcamp.com" + "/album/404"
	// to remove:
	// 		" + ".
	re := regexp.MustCompile(`(url: ".+)" \+ "(.+",)`)

	albumJSON = re.ReplaceAllString(albumJSON, "$1$2")

	return albumJSON
}

type Album struct {
	Current struct {
		Title string
	}
	Artist      string
	ReleaseDate string  `json:"album_release_date"`
	ArtworkID   int     `json:"art_id"`
	Tracks      []Track `json:"trackinfo"`
}

type Track struct {
	Number int `json:"track_num"`
	Title  string
	File   struct {
		MP3128 string `json:"mp3-128"`
	}
}

func downloadAlbum(url string, rootPath string) {
	logrus.WithField("URL", url).Info("Downloading album page")

	resp, err := http.Get(url)
	if err != nil {
		logrus.WithError(err).WithField("url", url).
			Error("Failed to download page")
		return
	}

	logrus.Info("Reading album page body")

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to read body")
	}

	body := string(bodyBytes)

	logrus.Info("Extracting album json from page body")

	albumJSON, err := extractAlbumJSON(body)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to extract album data")
	}

	var album Album

	logrus.Info("Unmarshaling album JSON")
	logrus.Debug(albumJSON)

	err = json5.Unmarshal([]byte(albumJSON), &album)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to unmarshal album JSON")
	}

	logrus.Info("Checking everything is ok")

	if album.Current.Title == "" {
		logrus.WithField("Album.Artist", album.Artist).
			Fatal("Album without title detected")
	}

	if album.Artist == "" {
		logrus.WithField("Album.Current.Title", album.Current.Title).
			Fatal("Album without artist detected")
	}

	if len(album.Tracks) == 0 {
		logrus.WithFields(logrus.Fields{
			"Album.Artist":        album.Artist,
			"Album.Current.Title": album.Current.Title,
		}).Info("Album without tracks detected")
		return
	}

	var albumYear string

	if album.ReleaseDate != "" {
		releaseTime, err := time.Parse("02 Jan 2006 15:04:05 MST", album.ReleaseDate)
		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"Album.Artist":        album.Artist,
				"Album.Current.Title": album.Current.Title,
				"Album.ReleaseDate":   album.ReleaseDate,
			}).Fatal("Failed to parse album release date")
			albumYear = strconv.Itoa(releaseTime.Year())
		}
	}

	albumPath := path.Join(rootPath, album.Artist,
		strings.TrimSpace(albumYear+" "+album.Current.Title))

	logrus.WithField("path", albumPath).Info("Creating album path")

	os.MkdirAll(albumPath, 0755)

	for _, t := range album.Tracks {
		if t.Number == 0 && len(album.Tracks) == 1 {
			t.Number = 1
		}

		if t.File.MP3128 == "" {
			logrus.WithFields(logrus.Fields{
				"Album.Artist":        album.Artist,
				"Album.Current.Title": album.Current.Title,
				"Track.Number":        t.Number,
				"Track.Title":         t.Title,
			}).Fatal("Track without MP3128 detected")
		}

		resp, err := http.Get(t.File.MP3128)
		if err != nil {
			logrus.WithField("Track.File.MP3128", t.File.MP3128).
				Fatal("Failed to download track MP3 file")
		}
		defer resp.Body.Close()

		filePath := path.Join(albumPath, strconv.Itoa(t.Number)+" "+t.Title+".mp3")

		logrus.WithField("path", filePath).Info("Creating track file")

		out, err := os.Create(filePath)
		if err != nil {
			logrus.WithField("filePath", filePath).Fatal("Failed to open track file")
		}

		logrus.WithField("URL", t.File.MP3128).Info("Downloading track")

		_, err = io.Copy(out, resp.Body)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"Album.Artist":        album.Artist,
				"Album.Current.Title": album.Current.Title,
				"Track.Number":        t.Number,
				"Track.Title":         t.Title,
			}).Fatal("Failed to copy track from body to file")
		}

		out.Close()

		if albumYear == "" {
			albumYear = "0"
		}

		// We depending on external tool eyeD3. Install it using `apt install eyed3`.

		cmd := exec.Command("/bin/sh", "-c", "command -v eyeD3")
		if err := cmd.Run(); err != nil {
			// We do not have eyeD3 installed so we can't tag mp3. Skipping it.
			logrus.Info("Can't tag mp3 due to absence of eyeD3 app, skipping")
			continue
		}

		logrus.Info("Tagging mp3 file using eyeD3 app")

		cmd = exec.Command("eyeD3",
			"-a", album.Artist,
			"-A", album.Current.Title,
			"-Y", albumYear,
			"-n", strconv.Itoa(t.Number),
			"-N", strconv.Itoa(len(album.Tracks)),
			"-t", t.Title,
			"-c", "Downloaded by bandcamp-download",
			filePath)

		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		err = cmd.Run()
		if err != nil {
			logrus.WithError(err).Error("Failed to run eyeD3 tagger")
			logrus.Error(stderr.String())
		}
	}

	logrus.Info("Downloading artwork")

	coverURL := "https://f4.bcbits.com/img/a" + strconv.Itoa(album.ArtworkID) + "_10.jpg"

	resp, err = http.Get(coverURL)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"Album.Artist":        album.Artist,
			"Album.Current.Title": album.Current.Title,
			"coverURL":            coverURL,
		}).Error("Failed to download album cover")
		return
	}
	defer resp.Body.Close()

	coverPath := path.Join(albumPath, "cover.jpg")

	logrus.WithField("path", coverPath).Info("Creating album cover file")

	out, err := os.Create(coverPath)
	if err != nil {
		logrus.WithField("coverPath", coverPath).Error("Failed to open album cover file")
		return
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"Album.Artist":        album.Artist,
			"Album.Current.Title": album.Current.Title,
			"coverURL":            coverURL,
		}).Error("Failed to copy album cover from body to file")
	}

}

func downloadAlbums(url string, rootPath string) {
	logrus.WithField("URL", url).Info("Downloading albums page")

	resp, err := http.Get(url)
	if err != nil {
		logrus.WithError(err).WithField("url", url).
			Error("Failed to download albums page")
		return
	}

	logrus.Info("Reading albums page body")

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to read body")
	}

	body := string(bodyBytes)

	logrus.Info("Extracting albums URLs from page body")

	re := regexp.MustCompile(`band_url = "(.*)"`)

	match := re.FindStringSubmatch(body)

	if len(match) == 0 {
		logrus.Fatal("Can't find artist URL")
	}

	artistURL := re.ReplaceAllString(match[0], "$1")

	re = regexp.MustCompile(`href="(/(album|track)/.*?)"`)

	matches := re.FindAllStringSubmatch(string(body), -1)

	if len(match) == 0 {
		logrus.Fatal("No album slugs found")
	}

	for _, m := range matches {
		albumSlug := re.ReplaceAllString(m[0], "$1")
		albumURL := artistURL + albumSlug

		logrus.WithField("albumURL", albumURL).Info("Downloading album")

		downloadAlbum(albumURL, rootPath)
	}
}

func downloadAlbumAction(c *cli.Context) error {
	url := c.String("url")
	rootPath := c.String("path")
	verbose := c.Bool("verbose")

	if verbose {
		logrus.SetLevel(logrus.DebugLevel)
	}

	downloadAlbum(url, rootPath)

	return nil
}

func downloadAlbumsAction(c *cli.Context) error {
	url := c.String("url")
	rootPath := c.String("path")
	verbose := c.Bool("verbose")

	if verbose {
		logrus.SetLevel(logrus.DebugLevel)
	}

	downloadAlbums(url, rootPath)

	return nil
}
