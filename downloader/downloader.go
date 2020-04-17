package downloader

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
)

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

func downloadTrack(albumPath string, a album, albumYear string, t track) {

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
		}).Error("Track without MP3128 detected")
		return
	}

	logrus.WithField("URL", t.File.MP3128).
		Info("Downloading track")

	resp, err := http.Get(t.File.MP3128)
	if err != nil {
		logrus.WithField("track.File.MP3128", t.File.MP3128).
			Error("Failed to download track MP3 file")
		return
	}
	defer func() {
		err = resp.Body.Close()
		if err != nil {
			logrus.WithError(err).Warn(
				"Failed to close downloading track body")
		}
	}()

	/*
		Creating track's mp3 file.
	*/

	fileName := fmt.Sprintf("%02d %s.mp3", t.Number, t.Title)
	fileName = strings.Replace(fileName, "/", "-", -1)

	filePath := path.Join(albumPath, fileName)

	logrus.WithField("path", filePath).Info("Creating track file")

	out, err := os.Create(filePath)
	if err != nil {
		logrus.WithField("filePath", filePath).
			Error("Failed to open track file")
		return
	}

	/*
		Copying track's downloaded body to created mp3 file.
	*/

	logrus.Info("Copying track's downloaded body to created mp3 file")

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"album.Artist":        a.Artist,
			"album.Current.Title": a.Current.Title,
			"track.Number":        t.Number,
			"track.Title":         t.Title,
		}).Error("Failed to copy track from body to file")
		return
	}

	err = out.Close()
	if err != nil {
		logrus.WithError(err).Warn("Failed to close track file")
	}

	/*
		Tagging track's mp3 file with metadata: artist, album, year, etc...
	*/

	logrus.Info("Tagging mp3 file with metadata")

	mp3, err := id3v2.Open(filePath, id3v2.Options{})
	if err != nil {
		logrus.WithError(err).WithField("filePath", filePath).Error(
			"Failed to open mp3 file")
		return
	}
	defer func() {
		err = mp3.Close()
		if err != nil {
			logrus.WithError(err).
				Warn("Failed to close mp3 file")
		}
	}()

	mp3.SetVersion(4)

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
		return
	}
}

func DownloadAlbum(url string, rootPath string) {

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
		logrus.WithError(err).Error("Failed to read body")
		return
	}

	body := string(bodyBytes)

	logrus.Info("Extracting album json from page body")

	albumJSON, err := extractAlbumJSON(body)
	if err != nil {
		logrus.WithError(err).Error("Failed to extract album data")
		return
	}

	var a album

	logrus.Info("Unmarshaling album JSON")

	err = json5.Unmarshal([]byte(albumJSON), &a)
	if err != nil {
		logrus.WithError(err).Error("Failed to unmarshal album JSON")
		return
	}

	/*
		Checking everything is ok.
	*/

	logrus.Info("Checking everything is ok")

	if a.Current.Title == "" {
		logrus.WithField("album.Artist", a.Artist).
			Error("Album without title detected")
		return
	}

	if a.Artist == "" {
		logrus.WithField("album.Current.Title", a.Current.Title).
			Error("Album without artist detected")
		return
	}

	if len(a.Tracks) == 0 {
		logrus.WithFields(logrus.Fields{
			"album.Artist":        a.Artist,
			"album.Current.Title": a.Current.Title,
		}).Error("Album without tracks detected")
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
			}).Error("Failed to parse album release date")
			return
		}
		albumYear = strconv.Itoa(releaseTime.Year())
	}

	/*
		Creating artist and album paths.
	*/

	albumPath := path.Join(rootPath, a.Artist,
		strings.TrimSpace(albumYear+" "+a.Current.Title))

	logrus.WithField("path", albumPath).Info("Creating album path")

	err = os.MkdirAll(albumPath, 0755)
	if err != nil {
		logrus.WithError(err).Error("Failed to create album path")
		return
	}

	/*
		Downloading tracks.
	*/

	for _, t := range a.Tracks {
		downloadTrack(albumPath, a, albumYear, t)
	}

	/*
		Downloading album cover.
	*/

	logrus.Info("Downloading album cover")

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
	defer func() {
		err = resp.Body.Close()
		if err != nil {
			logrus.WithError(err).Warn(
				"Failed to close album cover body")
		}
	}()

	coverPath := path.Join(albumPath, "cover.jpg")

	logrus.WithField("path", coverPath).
		Info("Creating album cover file")

	out, err := os.Create(coverPath)
	if err != nil {
		logrus.WithField("coverPath", coverPath).
			Error("Failed to open album cover file")
		return
	}
	defer func() {
		err = out.Close()
		if err != nil {
			logrus.WithError(err).Warn(
				"Failed to close album cover file")
		}
	}()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"album.Artist":        a.Artist,
			"album.Current.Title": a.Current.Title,
			"coverURL":            coverURL,
		}).Error("Failed to copy album cover from body to file")
	}
}

func DownloadAlbums(url string, rootPath string) {
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
		logrus.WithError(err).Error("Failed to read body")
		return
	}

	body := string(bodyBytes)

	logrus.Info("Extracting albums URLs from page body")

	re := regexp.MustCompile(`band_url = "(.*)"`)

	match := re.FindStringSubmatch(body)

	if len(match) == 0 {
		logrus.Error("Can't find artist URL")
		return
	}

	artistURL := re.ReplaceAllString(match[0], "$1")

	re = regexp.MustCompile(`href="(/(album|track)/.*?)"`)

	matches := re.FindAllStringSubmatch(string(body), -1)

	if len(match) == 0 {
		logrus.Error("No album slugs found")
		return
	}

	for _, m := range matches {
		albumSlug := re.ReplaceAllString(m[0], "$1")
		albumURL := artistURL + albumSlug

		logrus.WithField("albumURL", albumURL).
			Info("Downloading album")

		DownloadAlbum(albumURL, rootPath)
	}
}
