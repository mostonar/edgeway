module meep-dai-mgr/meepdaimgr

go 1.12

require (
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger v0.0.0-20211214133749-f203f7ab4f1c
	github.com/google/uuid v1.3.0
	github.com/lib/pq v1.10.6
	github.com/spf13/cobra v1.5.0
	github.com/hashicorp/go-memdb v1.3.4
)

replace (
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger => ../../go-packages/meep-logger
)
