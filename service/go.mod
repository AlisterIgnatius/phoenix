module github.com/wikimedia/phoenix/service

go 1.14

replace github.com/wikimedia/phoenix/storage => ../storage

require (
	github.com/aws/aws-sdk-go v1.34.25
	github.com/gorilla/handlers v1.5.1
	github.com/graph-gophers/graphql-go v0.0.0-20200819123640-3b5ddcd884ae
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/rs/cors v1.7.0
	github.com/wikimedia/phoenix/common v0.0.0-20200915234838-9cdaa9ab5421
	github.com/wikimedia/phoenix/storage v0.0.0-20200915234838-9cdaa9ab5421
)
