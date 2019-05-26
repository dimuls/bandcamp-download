package main

import (
	"os"

	"github.com/dimuls/bandcamp-download/downloader"

	"github.com/sirupsen/logrus"
	"gopkg.in/urfave/cli.v1"
)

func main() {
	app := cli.NewApp()
	app.Name = "bandcamp-download"
	app.Usage = ""
	app.Description = "Tool for download album or albums from bandcamp.com."
	app.Author = "Vadim Chernov"
	app.Email = "dimuls@yandex.ru"
	app.Version = "0.3.2"

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

func downloadAlbumAction(c *cli.Context) error {
	url := c.String("url")
	rootPath := c.String("path")
	verbose := c.Bool("verbose")

	if verbose {
		logrus.SetLevel(logrus.DebugLevel)
	}

	downloader.DownloadAlbum(url, rootPath)

	return nil
}

func downloadAlbumsAction(c *cli.Context) error {
	url := c.String("url")
	rootPath := c.String("path")
	verbose := c.Bool("verbose")

	if verbose {
		logrus.SetLevel(logrus.DebugLevel)
	}

	downloader.DownloadAlbums(url, rootPath)

	return nil
}
