module github.com/InterDigitalInc/AdvantEDGE/go-apps/meep-federation

go 1.12

require (
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-app-support-client v0.0.0
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-data-key-mgr v0.0.0
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-data-model v0.0.0 // indirect
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-federation-mgr v0.0.0
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-gis-engine-client v0.0.0-20230530211153-8f6edbeaf367 // indirect
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-http-logger v0.0.0
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger v0.0.0
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-metrics v0.0.0
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-model v0.0.0
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-mq v0.0.0
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-redis v0.0.0
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-sandbox-ctrl-client v0.0.0
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-service-mgmt-client v0.0.0
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-subscriptions v0.0.0
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-swagger-api-mgr v0.0.0
	github.com/antihax/optional v1.0.0 // indirect
	github.com/google/uuid v1.6.0
	github.com/gorilla/handlers v1.5.1
	github.com/gorilla/mux v1.8.0
	github.com/prometheus/client_golang v1.9.0
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d // indirect
)

replace (
	github.com/InterDigitalInc/AdvantEDGE/go-apps/meep-federation => ../../go-apps/meep-federation
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-app-support-client => ../../go-packages/meep-app-support-client
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-data-key-mgr => ../../go-packages/meep-data-key-mgr
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-data-model => ../../go-packages/meep-data-model
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-federation-mgr => ../../go-packages/meep-federation-mgr
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-http-logger => ../../go-packages/meep-http-logger
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger => ../../go-packages/meep-logger
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-metrics => ../../go-packages/meep-metrics
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-model => ../../go-packages/meep-model
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-mq => ../../go-packages/meep-mq
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-redis => ../../go-packages/meep-redis
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-sandbox-ctrl-client => ../../go-packages/meep-sandbox-ctrl-client
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-service-mgmt-client => ../../go-packages/meep-service-mgmt-client
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-subscriptions => ../../go-packages/meep-subscriptions
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-swagger-api-mgr => ../../go-packages/meep-swagger-api-mgr
	github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-websocket => ../../go-packages/meep-websocket
)
