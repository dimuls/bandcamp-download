# bandcamp-download

Download your favorite artist albums or a single album using `bandcamp-download`.

## `eyeD3` dependency

I couldn't find id3 v2.4 tagger library on go, so i decided to use external tool.
You need to install eyeD3:
```bash
apt install eyed3
```

But if you do not need tagging you can skip this.

## How to install?

You need `go` to build from the scratch:

```go
go install github.com/dimuls/bandcamp-download
```

Or you can download binary it from [releases page](https://github.com/dimuls/bandcamp-download/releases) and put it to your PATH.

## How to use?

Just use help:
```
./bandcamp-download -h
NAME:
   bandcamp-download - A new cli application

USAGE:
   bandcamp-download [global options] command [command options] [arguments...]

VERSION:
   0.1

AUTHOR:
   Vadim Chernov <dimuls@yandex.ru>

COMMANDS:
     album, a    download album from album page
     albums, as  download albums from albums page
     help, h     Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --help, -h     show help
   --version, -v  print the version
```

Also available:
```bash
./bandcamp-download album -h
./bandcamp-download albums -h
```

## Have any problems?

Feel free to contact me:
* email: dimuls@yandex.ru
* telegram: @dimuls