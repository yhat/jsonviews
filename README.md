# jsonviews

Filter out parts of JSON objects without reading them into memory first.

## Why?

Go's standard JSON encoding package __always buffers the entire JSON data__
before doing any unmarshalling. This means that unused fields are always read
into memory even if you know ahead of time that you don't need some of them.
If a field is particularly large, you're out of luck.

jsonviews attempts to remedy this by filtering out fields as the data is
requested. Reads from a View are paired with JSON validation and filtering,
requiring only a small amount of buffering.

## Example

```go
package main

import (
	"io"
	"os"
	"strings"

	"github.com/yhat/jsonviews"
)

func main() {
	reader := strings.NewReader(data) // some sort of io.Reader
	v := jsonviews.NewView(reader)
	v.AddFilter(".menu.id")
	v.AddFilter(".menu.value")
	io.Copy(os.Stdout, v)
}

var data = `{
  "menu": {
    "id": "file",
    "value": "File",
    "popup": {
      "menuitem": [
	    { "value": "New", "onclick": "CreateNewDoc()" },
        { "value": "Open", "onclick": "OpenDoc()" }
	  ]
    }
  }
}`
```

Output:

(pretty print not implemented yet)

```json
{
  "menu": {
    "id": "file",
    "value": "File"
  }
}
```
