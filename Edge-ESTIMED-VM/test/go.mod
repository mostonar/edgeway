module github.com/InterDigitalInc/AdvantEDGE/test/system

go 1.12

require (
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-gis-engine-client v0.0.0
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger v0.0.0-00010101000000-000000000000
	github.com/ghodss/yaml v1.0.0
	github.com/gorilla/handlers v1.4.0
	github.com/gorilla/mux v1.7.4
	gopkg.in/yaml.v2 v2.2.2
)

replace (
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-gis-engine-client => ../go-packages/meep-gis-engine-client
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger => ../go-packages/meep-logger
)
