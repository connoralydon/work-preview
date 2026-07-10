package previewv1

//go:generate sh -c "cd ../.. && protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative api/v1/preview.proto"
