module github.com/joec4i/emulators/storage

go 1.15

require (
	cloud.google.com/go/storage v1.10.0
	github.com/bluele/gcache v0.0.2
	github.com/fullstorydev/emulators/storage v0.0.0-00010101000000-000000000000
	github.com/google/btree v1.0.1
	golang.org/x/oauth2 v0.0.0-20210514164344-f6687ab2804c
	google.golang.org/api v0.47.0
	google.golang.org/protobuf v1.26.0
	gotest.tools/v3 v3.0.3
)

replace github.com/fullstorydev/emulators/storage => github.com/joec4i/emulators/storage v0.0.0-20230517060139-75a3881b0ac8
