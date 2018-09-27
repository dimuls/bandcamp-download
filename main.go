package main

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bogem/id3v2"
	"github.com/sirupsen/logrus"
	"github.com/yosuke-furukawa/json5/encoding/json5"
	"gopkg.in/urfave/cli.v1"
)

func main() {
	app := cli.NewApp()
	app.Name = "bandcamp-download"
	app.Usage = ""
	app.Description = "Tool for download album or albums from bandcamp.com."
	app.Author = "Vadim Chernov"
	app.Email = "dimuls@yandex.ru"
	app.Version = "0.1.1"

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

type album struct {
	Current struct {
		Title string
	}
	Artist      string
	ReleaseDate string  `json:"album_release_date"`
	ArtworkID   int     `json:"art_id"`
	Tracks      []track `json:"trackinfo"`
}

type track struct {
	Number int `json:"track_num"`
	Title  string
	File   struct {
		MP3128 string `json:"mp3-128"`
	}
}

// id3 v2.4 track frame ID.
const trackFrameID = "TRCK"

func downloadAlbum(url string, rootPath string) {

	/*
		Downloading album page and parsing it.
	*/

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

	var a album

	logrus.Info("Unmarshaling album JSON")
	logrus.Debug(albumJSON)

	err = json5.Unmarshal([]byte(albumJSON), &a)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to unmarshal album JSON")
	}

	/*
		Checking everything is ok.
	*/

	logrus.Info("Checking everything is ok")

	if a.Current.Title == "" {
		logrus.WithField("album.Artist", a.Artist).
			Fatal("Album without title detected")
	}

	if a.Artist == "" {
		logrus.WithField("album.Current.Title", a.Current.Title).
			Fatal("Album without artist detected")
	}

	if len(a.Tracks) == 0 {
		logrus.WithFields(logrus.Fields{
			"album.Artist":        a.Artist,
			"album.Current.Title": a.Current.Title,
		}).Info("Album without tracks detected")
		return
	}

	/*
		Preparing album year string.
	*/

	var albumYear string

	if a.ReleaseDate != "" {
		releaseTime, err := time.Parse("02 Jan 2006 15:04:05 MST",
			a.ReleaseDate)
		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"album.Artist":        a.Artist,
				"album.Current.Title": a.Current.Title,
				"album.ReleaseDate":   a.ReleaseDate,
			}).Fatal("Failed to parse album release date")
		}
		albumYear = strconv.Itoa(releaseTime.Year())
	}

	/*
		Creating artist and album paths.
	*/

	albumPath := path.Join(rootPath, a.Artist,
		strings.TrimSpace(albumYear+" "+a.Current.Title))

	logrus.WithField("path", albumPath).Info("Creating album path")

	os.MkdirAll(albumPath, 0755)

	/*
		Working with tracks.
	*/

	for _, t := range a.Tracks {

		// Sometimes we can get track number 0. Check if single track in album
		// and set track number to 1 if so.
		if t.Number == 0 && len(a.Tracks) == 1 {
			t.Number = 1
		}

		/*
			Downloading track.
		*/

		if t.File.MP3128 == "" {
			logrus.WithFields(logrus.Fields{
				"album.Artist":        a.Artist,
				"album.Current.Title": a.Current.Title,
				"track.Number":        t.Number,
				"track.Title":         t.Title,
			}).Fatal("Track without MP3128 detected")
		}

		resp, err := http.Get(t.File.MP3128)
		if err != nil {
			logrus.WithField("track.File.MP3128", t.File.MP3128).
				Fatal("Failed to download track MP3 file")
		}
		defer resp.Body.Close()

		/*
			Creating track's mp3 file.
		*/

		filePath := path.Join(albumPath, strconv.Itoa(t.Number)+" "+
			t.Title+".mp3")

		logrus.WithField("path", filePath).Info("Creating track file")

		out, err := os.Create(filePath)
		if err != nil {
			logrus.WithField("filePath", filePath).
				Fatal("Failed to open track file")
		}

		logrus.WithField("URL", t.File.MP3128).Info("Downloading track")

		/*
			Copy track's  downloaded body to created mp3 file.
		*/

		_, err = io.Copy(out, resp.Body)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"album.Artist":        a.Artist,
				"album.Current.Title": a.Current.Title,
				"track.Number":        t.Number,
				"track.Title":         t.Title,
			}).Fatal("Failed to copy track from body to file")
		}

		out.Close()

		/*
			Taggin track's mp3 file with metadata: artist, album, year, etc...
		*/

		logrus.Info("Tagging mp3 file with metadata")

		mp3, err := id3v2.Open(filePath, id3v2.Options{})
		if err != nil {
			logrus.WithError(err).WithField("filePath", filePath).Error(
				"Failed to open mp3 file")
			continue
		}

		mp3.SetArtist(a.Artist)
		mp3.SetAlbum(a.Current.Title)
		mp3.SetTitle(t.Title)

		if albumYear != "" {
			mp3.SetYear(albumYear)
		}

		mp3.AddCommentFrame(id3v2.CommentFrame{
			Encoding: id3v2.EncodingUTF8,
			Language: "eng",
			Text:     "Downloaded by bandcamp-download",
		})

		mp3.AddFrame(trackFrameID, id3v2.TextFrame{Encoding: id3v2.EncodingUTF8,
			Text: fmt.Sprintf("%d/%d", t.Number, len(a.Tracks))})

		err = mp3.Save()
		if err != nil {
			logrus.WithError(err).Error("Failed to save tagged mp3 file")
			continue
		}
	}

	/*
		Downloading album cover.
	*/

	logrus.Info("Downloading artwork")

	coverURL := "https://f4.bcbits.com/img/a" +
		strconv.Itoa(a.ArtworkID) + "_10.jpg"

	resp, err = http.Get(coverURL)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"album.Artist":        a.Artist,
			"album.Current.Title": a.Current.Title,
			"coverURL":            coverURL,
		}).Error("Failed to download album cover")
		return
	}
	defer resp.Body.Close()

	coverPath := path.Join(albumPath, "cover.jpg")

	logrus.WithField("path", coverPath).
		Info("Creating album cover file")

	out, err := os.Create(coverPath)
	if err != nil {
		logrus.WithField("coverPath", coverPath).
			Error("Failed to open album cover file")
		return
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"album.Artist":        a.Artist,
			"album.Current.Title": a.Current.Title,
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
