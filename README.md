# ProtoSync synchronises remote .proto files to a local directory

This tool syncs the transitive import closure of a set of .proto files to a local
directory. A configuration file tells protosync where to retrieve .proto files from. It
then retrieves and/or parses the .proto files specified on the command-line, recursively
retrieving all imports.

## What problem does this solve?

Unlike most modern languages, Protobufs does not have a packaging system. The typical
solution to using third party Protobufs then becomes copying those .proto files into 
your source. This tool automates that process by recursively parsing and resolving 
imports from third party, or your own .proto files.

## Contributing

Code is always welcome, but so to are extra `repo` entries in the builtin config. The
more repo entries are built in, the more .proto files can be resolved by default!

## Example

For example, if we create the following in `protos/service.proto`:

```protobuf
syntax = "proto3";

package service;

import "google/api/annotations.proto";
import "google/rpc/status.proto"; // Imported for API doc references.
import "protoc-gen-swagger/options/annotations.proto";
```

The following will recursively retrieve all remote imports referenced in the
local proto root `./protos` as well as `google/api/http.proto`, and place them in
`./third_party/protos`.

    $ protosync -I./protos --dest=./third_party/protos google/api/http.proto
    info: https://raw.githubusercontent.com/googleapis/googleapis/master/google/api/http.proto -> /Users/alec/Projects/protosync/third_party/protos/google/api/http.proto
    info: https://raw.githubusercontent.com/googleapis/googleapis/master/google/api/annotations.proto -> /Users/alec/Projects/protosync/third_party/protos/google/api/annotations.proto
    info: https://raw.githubusercontent.com/protocolbuffers/protobuf/master/src/google/protobuf/descriptor.proto -> /Users/alec/Projects/protosync/third_party/protos/google/protobuf/descriptor.proto
    info: https://raw.githubusercontent.com/googleapis/googleapis/master/google/rpc/status.proto -> /Users/alec/Projects/protosync/third_party/protos/google/rpc/status.proto
    info: https://raw.githubusercontent.com/protocolbuffers/protobuf/master/src/google/protobuf/any.proto -> /Users/alec/Projects/protosync/third_party/protos/google/protobuf/any.proto
    info: https://raw.githubusercontent.com/grpc-ecosystem/grpc-gateway/v1.15.2/protoc-gen-swagger/options/annotations.proto -> /Users/alec/Projects/protosync/third_party/protos/protoc-gen-swagger/options/annotations.proto
    info: https://raw.githubusercontent.com/grpc-ecosystem/grpc-gateway/v1.15.2/protoc-gen-swagger/options/openapiv2.proto -> /Users/alec/Projects/protosync/third_party/protos/protoc-gen-swagger/options/openapiv2.proto
    info: https://raw.githubusercontent.com/protocolbuffers/protobuf/master/src/google/protobuf/struct.proto -> /Users/alec/Projects/protosync/third_party/protos/google/protobuf/struct.proto

## Usage

For simple use cases `protosync` can be used standalone, but for more complex situations 
it also supports a HCL configuration file. Run `protosync --help` to see the schema 
for the configuration file as well as command-line usage.

## Customising

The `protosync` command-line tool is a thin wrapper around an extensible API. Look 
at the `resolver` package to see example implementations of how to extend `protosync`.

## Why doesn't this use git clone?

As the above example illustrates, `protosync` attempts to directly retrieve
protos via HTTP rather than cloning via git. This is primarily an
optimisation due to the use of large monorepos at Square, where cloning down
hundreds of megabytes of source to retrieve one or two files was
unreasonable. That said, a git-based resolver will be added.

## Development

Protosync uses [hermit](https://cashapp.github.io/hermit/) for uniform
tooling. Just clone this repo, activate hermit and you are ready to
build, test and lint:

    . ./bin/activate-hermit
    go build ./cmd/protosync
    go test ./...
    golangci-lint run

## License

Copyright 2021 Square, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
