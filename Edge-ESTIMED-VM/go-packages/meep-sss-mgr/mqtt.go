package sssmgr

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"

	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type SssMgrMqtt struct {
	running    bool
	opts       *mqtt.ClientOptions
	client     mqtt.Client
	sent_topic string
	notify     func(map[string]interface{})
	respMu     sync.Mutex
	pending    map[uint16]chan mqtt.Message
	responses  map[uint16]mqtt.Message
}

func onMessageReceived(client mqtt.Client, msg mqtt.Message) {
	go func() {
		log.Info("onMessageReceived: Received message: ", msg.Payload(), " on topic ", msg.Topic())
		// if _notify != nil {
		// 	...
		// } else {
		// 	log.Info("onMessageReceived: null pointer for the callback")
		// }
	}()
}

func onMessageReceivedReq(client mqtt.Client, msg mqtt.Message) {
	go func() {
		log.Info("onMessageReceivedReq: Received message: ", string(msg.Payload()), " on topic ", msg.Topic())
		// if _notify != nil {
		// 	...
		// } else {
		// 	log.Info("onMessageReceivedReq: null pointer for the callback")
		// }
	}()
}

func (broker_mqtt *SssMgrMqtt) onMessageReceivedResp(client mqtt.Client, msg mqtt.Message) {
	go func() {
		log.Info("onMessageReceivedResp: Received message: ", string(msg.Payload()), " on topic ", msg.Topic())
		broker_mqtt.respMu.Lock()
		defer broker_mqtt.respMu.Unlock()
		if ch, ok := broker_mqtt.pending[msg.MessageID()]; ok {
			delete(broker_mqtt.pending, msg.MessageID())
			ch <- msg
			return
		}
		if broker_mqtt.responses == nil {
			return
		}
		broker_mqtt.responses[msg.MessageID()] = msg
	}()
}

func (broker_mqtt *SssMgrMqtt) onMessageReceivedNot(client mqtt.Client, msg mqtt.Message) {
	go func() {
		log.Info("onMessageReceivedNot: Received message: ", string(msg.Payload()), " on topic ", msg.Topic())
		//{\"fr\": \"/laboai-acme-ic-cse\", \"to\": \"mqtt://172.29.10.56:1883\", \"ot\": \"20250327T134413,103223\", \"op\": 5, \"rqi\": \"6400213617020980613\", \"rvi\": \"4\", \"drt\": 1, \"pc\": {\"m2m:sgn\": {\"nev\": {\"net\": 3, \"rep\": {\"m2m:cin\": {\"cnf\": \"text/plain:0\", \"con\": \"toto\", \"rn\": \"1234\", \"ri\": \"cin3503344383965775257\", \"pi\": \"cnt1992620086833081496\", \"ct\": \"20250327T134413,093784\", \"lt\": \"20250327T134413,093784\", \"ty\": 4, \"cs\": 4, \"st\": 2, \"et\": \"20300326T134347,108295\"}}}, \"sur\": \"/laboai-acme-ic-cse/sub3169944238694067800\"}}} on topic /oneM2M/req/laboai-acme-ic-cse/mqtt:::172.29.10.56:1883/json"
		// Prepare response
		s := strings.Split(msg.Topic(), "/") // /oneM2M/req/laboai-acme-ic-cse/mqtt:::172.29.10.56:1883/json
		if len(s) < 6 {
			log.Warn("onMessageReceivedNot: unexpected topic: ", msg.Topic())
			return
		}
		resp_topic := "/oneM2M/resp/" + s[4] + "/" + s[3] + "/" + s[5]
		log.Debug("onMessageReceivedNot: resp_topic: ", resp_topic)
		if broker_mqtt.notify != nil {
			var d map[string]interface{}
			err := json.Unmarshal(msg.Payload(), &d)
			if err != nil {
				log.Warn("onMessageReceivedNot: ", err.Error())
				return
			}
			if pc, ok := d["pc"]; ok {
				log.Debug("onMessageReceivedNot: pc: ", pc)
				log.Debug("onMessageReceivedNot: TypeOf(pc): ", reflect.TypeOf(pc))
				if _, ok := d["pc"].(map[string]interface{}); !ok {
					log.Warn("onMessageReceivedNot: pc entry has an unexpected type")
					return
				}
				body := d["pc"].(map[string]interface{})
				if _, ok := body["m2m:sgn"]; !ok {
					log.Error("Only m2m:sgn is expected")
					return
				}
				if _, ok := body["m2m:sgn"].(map[string]interface{}); !ok {
					log.Warn("onMessageReceivedNot: m2m:sgn entry has an unexpected type")
					return
				}
				m := body["m2m:sgn"].(map[string]interface{})
				log.Info("handleRoot: m: ", m)
				broker_mqtt.notify(m)
			} else {
				log.Warn("onMessageReceivedNot: pc entry not found")
				return
			}
		} else {
			log.Debug("onMessageReceivedNot: null pointer for the callback")
		}
		// FIXME FSCOM Send the response
	}()
}

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	//log.Info("mqtt.OnConnectHandler: Connected")
}

var connectLostHandler mqtt.ConnectionLostHandler = func(client mqtt.Client, err error) {
	//log.Info("Connect lost:", err)
}

func NewSssMgrMqtt() (broker_mqtt *SssMgrMqtt) {
	log.Info(">>> NewSssMgrMqtt")
	return new(SssMgrMqtt)
}

func (broker_mqtt *SssMgrMqtt) init(tm *SssMgr, notify func(map[string]interface{})) (err error) {
	log.Info(">>> init")

	broker_mqtt.running = false
	broker_mqtt.notify = notify
	broker_mqtt.pending = make(map[uint16]chan mqtt.Message)
	broker_mqtt.responses = make(map[uint16]mqtt.Message)

	broker_mqtt.opts = mqtt.NewClientOptions()
	broker_mqtt.opts.SetDefaultPublishHandler(onMessageReceived)
	broker_mqtt.opts.SetClientID(fmt.Sprintf("meep-iot-%s-%d", tm.hostId, tm.port))
	broker_mqtt.opts.OnConnect = connectHandler
	broker_mqtt.opts.OnConnectionLost = connectLostHandler
	broker_mqtt.opts.CleanSession = true
	//broker_mqtt.opts.ResumeSubs = true
	broker_mqtt.opts.SetUsername("")
	broker_mqtt.opts.SetPassword("")
	log.Info("init: Add brocker: ", fmt.Sprintf("tcp://%s:%d", tm.host, tm.port))
	broker_mqtt.opts.AddBroker(fmt.Sprintf("tcp://%s:%d", tm.host, tm.port))
	broker_mqtt.client = mqtt.NewClient(broker_mqtt.opts)

	log.Info("init: Connect to MQTT server...")
	token := broker_mqtt.client.Connect()
	if token.Error() != nil {
		log.Error(token.Error())
		return token.Error()
	}
	token.Wait()

	// Subscribe
	// tp := "oneM2M/req/+/" + tm.cse_name + "/#"
	// log.Info("init: Subscribe to: ", tp)
	// token = broker_mqtt.client.Subscribe(tp, 0, onMessageReceivedReq) // qos:0
	// if token.Error() != nil {
	// 	log.Error(token.Error())
	// 	return token.Error()
	// }
	// token.Wait()
	tp := "/oneM2M/resp/+/" + tm.cse_name + "/#"
	log.Info("init: Subscribe to: ", tp)
	token = broker_mqtt.client.Subscribe(tp, 0, broker_mqtt.onMessageReceivedResp) // qos:0
	if token.Error() != nil {
		broker_mqtt.client.Disconnect(250)
		log.Error(token.Error())
		return token.Error()
	}
	token.Wait()
	tp = "/oneM2M/req/" + tm.cse_name + "/+/#"
	log.Info("init: Subscribe to: ", tp)
	token = broker_mqtt.client.Subscribe(tp, 1, broker_mqtt.onMessageReceivedNot) // qos:1
	if token.Error() != nil {
		broker_mqtt.client.Disconnect(250)
		log.Error(token.Error())
		return token.Error()
	}
	token.Wait()

	broker_mqtt.running = true
	log.Info("init: Client is connected")

	return nil
}

func (broker_mqtt *SssMgrMqtt) uninit() (err error) {
	log.Info(">>> uninit")

	if !broker_mqtt.running {
		err := errors.New("MQTT not initialized or diconnected")
		log.Error(err.Error())
		return err
	}

	token := broker_mqtt.client.Unsubscribe("/oneM2M")
	if token.Error() != nil {
		log.Error(token.Error())
		// Continue
	}
	token.Wait()
	broker_mqtt.client.Disconnect(250)
	broker_mqtt.running = false

	broker_mqtt.respMu.Lock()
	broker_mqtt.pending = nil
	broker_mqtt.responses = nil
	broker_mqtt.notify = nil
	broker_mqtt.respMu.Unlock()

	return nil
}

func (broker_mqtt *SssMgrMqtt) send(p_ctx SssMgrBindingProtocolContext) (err error, resp interface{}) {
	log.Info(">>> send")

	// Sanity checks
	if !broker_mqtt.running {
		err := errors.New("MQTT not initialized or diconnected")
		log.Error(err.Error())
		return err, nil
	}

	// Prepare the topic
	broker_mqtt.sent_topic = "/oneM2M/req/" + "C" + p_ctx.from + "/" + p_ctx.hostId + "/json" // FIXME FSCOM Use parameter for type
	// Complete payload dictionary
	var body = map[string]interface{}{}
	body["op"] = p_ctx.op
	if err != nil {
		log.Error(err.Error())
		return err, nil
	}
	if p_ctx.ty != -1 {
		body["ty"] = p_ctx.ty
	}
	body["fr"] = "C" + p_ctx.from
	body["to"] = p_ctx.to
	body["rqi"] = p_ctx.rqi
	body["rvi"] = p_ctx.rvi[0]
	if p_ctx.body != nil {
		body["pc"] = p_ctx.body
	}
	if p_ctx.queries != nil && len(p_ctx.queries) != 0 {
		d := make(map[string]interface{}, 0)
		for k, v := range p_ctx.queries { // FIXME FSCOM Check in pure string is working too?
			if k == "ty" {
				s, err := strconv.Atoi(v)
				if err != nil {
					log.Error(err.Error())
					return err, nil
				}
				d[k] = []int{s}
				continue
			}
			d[k], err = strconv.Atoi(v)
			if err != nil {
				log.Error(err.Error())
				return err, nil
			}
		} // End of 'for' statement
		d["fo"] = 1
		body["fc"] = d
	}
	log.Debug("send: body=", body)

	content, err := json.Marshal(body)
	if err != nil {
		log.Error("send: ", err.Error())
		return err, nil
	}
	log.Debug("send: content: ", string(content))
	log.Debug("send: topic: ", broker_mqtt.sent_topic)

	token := broker_mqtt.client.Publish(broker_mqtt.sent_topic, 0, false, string(content))
	token.Wait()

	// Synchronization with the response. Match on this client's publish id so a
	// second platform cannot consume or panic on the first client's WaitGroup.
	messageID := token.(*mqtt.PublishToken).MessageID()
	broker_mqtt.respMu.Lock()
	if val, ok := broker_mqtt.responses[messageID]; ok {
		delete(broker_mqtt.responses, messageID)
		broker_mqtt.respMu.Unlock()
		return decodeMqttResponse(val)
	}
	ch := make(chan mqtt.Message, 1)
	if broker_mqtt.pending == nil {
		broker_mqtt.pending = make(map[uint16]chan mqtt.Message)
	}
	broker_mqtt.pending[messageID] = ch
	broker_mqtt.respMu.Unlock()

	log.Debug("send: Start waiting for the response")
	val := <-ch
	log.Debug("send: Got the response")
	return decodeMqttResponse(val)
}

func decodeMqttResponse(val mqtt.Message) (err error, resp interface{}) {
	log.Debug("send: Get the response: ", string(val.Payload()))
	var d map[string]interface{}
	err = json.Unmarshal(val.Payload(), &d)
	if err != nil {
		log.Error("send: ", err.Error())
		return err, nil
	}
	if r, ok := d["pc"]; ok {
		log.Debug("send: r: ", r)
		log.Debug("send: TypeOf(r): ", reflect.TypeOf(r))
		return nil, r
	}
	return nil, nil
}
