module github.com/cloudygreybeard/kql

go 1.21

require (
	github.com/cloudygreybeard/kqlparser v0.0.0-00010101000000-000000000000
	github.com/spf13/cobra v1.8.1
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
)

replace github.com/cloudygreybeard/kqlparser => ../kqlparser
