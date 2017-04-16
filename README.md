# rubymarshal

A [Ruby Marshal (version 4.8)](https://ruby-doc.org/core-2.3.0/Marshal.html) encoder/decoder in Golang. Why? Who knows.

```sh
go get github.com/samcday/rubymarshal
```

## Encoding

Arbitrary Golang structures can be encoded to equivalent Ruby types. Ruby Symbols, references to Ruby classes/modules, and instances of Ruby objects are supported.

Encoding is not as sophisticated as core packages like `encoding/json` - serializing arbitrary structs (and using struct tags for info) is not implemented. You can convert arbitrary structs to maps (which are supported here) using something like the fantastic [fatih/structs](https://github.com/fatih/structs).

```go
package main

import(
  "os"
  "github.com/samcday/rubymarshal"
)

func main() {
  b, err := rubymarshal.Encode(map[string]interface{}{
    "Test": []interface{}{123, "hello!", rubymarshal.Symbol("testsym")},
    "SomeClass": rubymarshal.Class("File::Stat"),
    "SomeModule": rubymarshal.Module("Process"),
    "AnInstance": rubymarshal.Instance{
      Name: "Gem::Version",
      UserMarshalled: true,
      Data: []string{"1.2.3"},
    },
  })

  if err != nil {
    panic(err)
  }

  os.Stdout.Write(b)
}
```

Save the example to `test.go` and run it like this:

```sh
$ go run test.go | ruby -e 'puts Marshal.load($stdin).inspect'
{"SomeModule"=>Process, "AnInstance"=>#<Gem::Version "1.2.3">, "Test"=>[123, "hello!", :testsym], "SomeClass"=>File::Stat}
```

### A note on strings

When encoding, if we encounter raw Go `string` types, we'll assume they're UTF-8 encoded. If a specific encoding needs to be used, you should wrap the string in an `IVar` (use the `NewEncodingIVar` helper) and specify the character encoding used.

## Useful links

 * http://jakegoulding.com/blog/2013/01/15/a-little-dip-into-rubys-marshal-format/
 * https://github.com/ruby/ruby/blob/trunk/marshal.c
