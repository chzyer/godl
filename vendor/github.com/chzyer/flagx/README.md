# flagx
A Replacement for golang stdlib flag

## Getting started

```{go}
// main.go
type Config struct {
	FileName string `flag:"[0]"`
}

func main() {
	var c Config
	flagx.Parse(&c)
	fmt.Println(c.FileName)
}
```

```bash
go run main.go ~/.profile
// Output: ~/.profile
```

