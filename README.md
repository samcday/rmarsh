# rmarsh

A [Ruby Marshal (version 4.8)](https://ruby-doc.org/core-2.3.0/Marshal.html) encoder/decoder in Golang. Why? Who knows.

```sh
go get github.com/samcday/rmarsh
```

This library sports low level Generator / Parser classes for high performance streaming access to the Marshal format. It also offers a higher level Mapper to marshal and unmarshal between Ruby and Go types.

Still under heavy development, no useful dox yet.

## Useful links

 * http://jakegoulding.com/blog/2013/01/15/a-little-dip-into-rubys-marshal-format/
 * https://github.com/ruby/ruby/blob/trunk/marshal.c
