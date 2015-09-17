# godl
a terminal multithreaded download-resumable (proxy) downloader

## build

```{shell}
go get github.com/chzyer/godl
```

## usage

```
$ ./godl
godl [option] <Url>

option:
  -b=20: block size represented by bit
  -f=false: overwritten if file is exists, false mean resume the progress from the meta file
  -meta=false: print meta
  -n=5: specified the max connections connected
  -p=true: show progress
  -s=: godl will enter server mode if specified listen addr with -s
  -u=: url
  -v=false: turn on debug mode
```
