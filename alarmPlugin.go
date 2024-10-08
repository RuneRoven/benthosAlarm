package benthosAlarm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redpanda-data/benthos/v4/public/service"
)

type alarms struct {
	json            string
	value           float64
	stringValue     string
	operator        string
	reset           float64
	resetOperator   string
	filterTime      time.Duration
	sendInterval    time.Duration
	alarmText       string
	alarmObject     string
	addToJson       bool
	alarmJsonStruct string
	sendAlarmOnly   bool
	addValue        bool
	cleanMsg        bool
	addMeta         bool
	startTime       time.Time // to track when the condition first becomes true
	lastTriggerTime time.Time
	trigger         bool
	alarmTriggered  bool
	timerTriggered  bool
	stopTickerChan  chan struct{}
	mu              sync.Mutex
	send            bool
	savedMsg        *service.Message
}

var alarmConf = service.NewConfigSpec().
	Summary("Creates an processor that sends data when conditions are met. Created by Daniel H").
	Description("This processor plugin enables Benthos to send data when specific conditions are met. " +
		"Configure the plugin by specifying the alarm value, reset value, operator and reset operator.").
	Field(service.NewFloatField("value").Description("Alarm value, default '100'").Default(100.0)).
	Field(service.NewStringField("json").Description("tag name is value is json)").Default("")).
	Field(service.NewStringField("stringValue").Description("Alarm value if using string)").Default("")).
	Field(service.NewStringField("operator").Description("allowed operators: <,>,=").Default(">")).
	Field(service.NewFloatField("reset").Description("reset value").Default(0.0)).
	Field(service.NewStringField("resetOperator").Description("reset operator").Default("<")).
	Field(service.NewStringField("filterTime").Description("Time filter before trigger. Default 0s").Default("0s")).
	Field(service.NewStringField("sendInterval").Description("Interval time before resending alarm. Default 0s").Default("0s")).
	Field(service.NewStringField("alarmText").Description("Alarm text to be added to the alarm. Default 'Alarm'").Default("Alarm")).
	Field(service.NewStringField("alarmObject").Description("Name of the json object for the alarm. Default 'alarm'").Default("alarm")).
	Field(service.NewBoolField("addToJson").Description("Add the alarm to the json structure").Default(false)).
	Field(service.NewStringField("alarmJsonStruct").Description("specific json struct for output. Default ''").Default("")).
	Field(service.NewBoolField("sendAlarmOnly").Description("Block all messages except alarm").Default(true)).
	Field(service.NewBoolField("addValue").Description("Add the current value to the alarm text").Default(false)).
	Field(service.NewBoolField("cleanMsg").Description("Create a new clean msg with alarmtext only").Default(true)).
	Field(service.NewBoolField("addMeta").Description("Add existing metadata to message").Default(false))

// ParseValue converts an interface{} to float64 or string
func ParseValue(val interface{}) (float64, string, error) {
	switch v := val.(type) {
	case float64:
		return v, "", nil
	case float32:
		return float64(v), "", nil
	case int:
		return float64(v), "", nil
	case int8:
		return float64(v), "", nil
	case int16:
		return float64(v), "", nil
	case int32:
		return float64(v), "", nil
	case int64:
		return float64(v), "", nil
	case uint:
		return float64(v), "", nil
	case uint8:
		return float64(v), "", nil
	case uint16:
		return float64(v), "", nil
	case uint32:
		return float64(v), "", nil
	case uint64:
		return float64(v), "", nil
	case string:
		// Check if it's a JSON string
		var data interface{}
		if err := json.Unmarshal([]byte(v), &data); err == nil {
			// Successfully parsed as JSON, handle it recursively
			return ParseValue(data)
		} else {
			// Not a valid JSON string, try to convert to float
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return f, "", nil
			} else {
				return 0, v, nil // Return as string if it's not a valid float
			}
		}
	case bool:
		if v {
			return 1.0, "", nil
		} else {
			return 0.0, "", nil
		}
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return f, "", nil
		} else {
			return 0, string(v), nil // Return as string if it's not a valid float
		}

	case map[string]interface{}:
		// Handle nested map (for JSON objects)
		// You may want to recurse or handle specific fields here
		//log.Println("Encountered nested map, need to handle it separately:", v)
		return 0, "", fmt.Errorf("unsupported type: %T", v)
	case []interface{}:
		// Handle slice (for JSON arrays)
		// You may want to iterate through elements or handle specific indices here
		log.Println("Encountered slice (array), need to handle it separately:", v)
		return 0, "", fmt.Errorf("unsupported type: %T", v)
	default:
		log.Printf("Unexpected type %T, value: %v\n", v, v)
		return 0, "", fmt.Errorf("unsupported type: %T", v)
	}
}
func convertToTime(durationStr string) (time.Duration, error) {
	// Regular expression to match the duration string
	re := regexp.MustCompile(`(\d+d)?(\d+h)?(\d+m)?(\d+s)?`)
	matches := re.FindStringSubmatch(durationStr)

	if matches == nil {
		return 0, fmt.Errorf("invalid duration format: %s", durationStr)
	}

	var totalDuration time.Duration

	for _, match := range matches[1:] {
		if match == "" {
			continue
		}
		// Split the match into numeric part and unit part
		unit := match[len(match)-1:]
		valueStr := match[:len(match)-1]
		value, err := strconv.Atoi(valueStr)
		if err != nil {
			return 0, err
		}
		switch unit {
		case "d":
			totalDuration += time.Duration(value) * 24 * time.Hour
		case "h":
			totalDuration += time.Duration(value) * time.Hour
		case "m":
			totalDuration += time.Duration(value) * time.Minute
		case "s":
			totalDuration += time.Duration(value) * time.Second
		}
	}
	return totalDuration, nil
}
func (a *alarms) parseConditions(operator string) string {
	switch operator {
	case "<":
		return "smaller than"
	case "<=":
		return "smaller or equal than"
	case ">":
		return "bigger than"
	case ">=":
		return "bigger or equal than"
	case "!=":
		return "not equal to"
	case "=":
		return "equal to"
	default:
		return ""
	}
}
func (a *alarms) conditionsMsg() string {
	msg := ""
	if a.stringValue != "" {
		msg = fmt.Sprintf("%s%s%s%s", "Alarm when value is equal to ", a.stringValue, " and reset when value is not equal to ", a.stringValue)

	} else {
		floatValue, _, _ := ParseValue(a.value)
		floatReset, _, _ := ParseValue(a.reset)
		msg = fmt.Sprintf("%s%s%s%f%s%s%s%f", "Alarm when value is ", a.parseConditions(a.operator), " ", floatValue, " and reset when value is ", a.parseConditions(a.resetOperator), " ", floatReset)
	}

	return msg
}
func (a *alarms) condition(operator string, value, threshold float64) (bool, error) {
	switch operator {
	case "<":
		return value < threshold, nil
	case "<=":
		return value <= threshold, nil
	case ">":
		return value > threshold, nil
	case ">=":
		return value >= threshold, nil
	case "!=":
		return value != threshold, nil
	case "=":
		return value == threshold, nil
	default:
		return false, fmt.Errorf("unknown operator: %s", operator)
	}
}
func extractValueFromPath(dataMap map[string]interface{}, path string) (float64, string, error) {
	keys := strings.Split(path, ".")
	var current interface{} = dataMap

	for _, key := range keys {
		if nextMap, ok := current.(map[string]interface{}); ok {
			current = nextMap[key]
		} else {
			return 0, "", fmt.Errorf("path %s does not exist in the JSON structure", path)
		}
	}

	return ParseValue(current)
}
func updateJSONAtPath(data map[string]interface{}, path string, newData map[string]interface{}) error {
	keys := strings.Split(path, ".")
	lastKey := keys[len(keys)-1]
	currentMap := data

	for _, key := range keys[:len(keys)-1] {
		if nextMap, exists := currentMap[key].(map[string]interface{}); exists {
			currentMap = nextMap
		} else {
			return fmt.Errorf("path %s does not exist in the JSON structure", path)
		}
	}

	currentMap[lastKey] = newData
	return nil
}

func (a *alarms) checkAndTriggerAlarm() {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Check if the alarm is initially triggered based on startTime and filterTime
	if !a.startTime.IsZero() {
		if time.Since(a.startTime) >= a.filterTime && !a.trigger && a.alarmTriggered {
			a.trigger = true
			a.send = true
			a.timerTriggered = true
		}
	}
	// If sendInterval is set and the alarm was initially triggered, check for re-triggering
	if a.sendInterval > 0 && a.trigger && a.alarmTriggered {
		now := time.Now()
		if a.lastTriggerTime.IsZero() {
			a.lastTriggerTime = now // ensure it is not double triggering
		}
		// Calculate elapsed time since the last trigger
		if now.Sub(a.lastTriggerTime) >= a.sendInterval {
			a.send = true
			a.timerTriggered = true
			a.lastTriggerTime = now
		}
	}
}
func (a *alarms) startTicker() {
	ticker := time.NewTicker(1000 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.checkAndTriggerAlarm()
		case <-a.stopTickerChan:
			return
		}
	}
}
func (a *alarms) createNewMsg(msgContent string, msgValue interface{}, dataMap map[string]interface{}, msg *service.Message) *service.Message {
	newMsg := service.NewMessage(nil)
	conditions := a.conditionsMsg()
	if a.cleanMsg {
		newMsg = service.NewMessage([]byte{})
		newMsg.SetStructured(map[string]interface{}{
			a.alarmObject:  a.send,
			"msg: ":        msgContent,
			"value: ":      msgValue,
			"conditions: ": conditions,
		})
		msg.MetaWalk(func(key, value string) error {
			dataMap[key] = value
			return nil
		})
	} else {
		if !a.addToJson {
			dataMap[a.alarmObject] = a.send
			dataMap["msg"] = msgContent
			dataMap["value"] = msgValue
			dataMap["conditions"] = conditions
		}
		if a.addMeta {
			// Extract metadata and add to dataMap
			msg.MetaWalk(func(key, value string) error {
				dataMap[key] = value
				return nil
			})
		}
		// Create a new message with the updated data
		if a.addToJson {
			log.Println("adding to json")
			jsonMsg := make(map[string]interface{})
			jsonMsg[a.alarmObject] = a.send
			jsonMsg["msg"] = msgContent
			jsonMsg["value"] = msgValue
			jsonMsg["conditions"] = conditions
			err := updateJSONAtPath(dataMap, a.alarmJsonStruct, jsonMsg)
			if err != nil {
				return nil
			}
		}
		newMsg = service.NewMessage(nil)
		newMsg.SetStructured(dataMap)
	}
	return newMsg
}
func (a *alarms) alarmCheck(floatValue float64, strValue string) (bool, bool, error) {
	var checkAlarm bool
	var checkReset bool
	var err error

	if a.stringValue != "" {
		// add some check here to only use the string if != ""
		if (strValue == a.stringValue && a.operator == "=") || (strValue != a.stringValue && a.operator == "!=") {
			checkAlarm = true
		}
	} else {
		checkAlarm, err = a.condition(a.operator, floatValue, a.value)
	}
	if err != nil {
		return false, false, err
	}

	// check if alarm should reset if the reset threshold i met
	if a.stringValue != "" {
		if (strValue != a.stringValue && a.operator == "=") || (strValue == a.stringValue && a.operator == "!=") {
			checkReset = true
		}
	} else {
		checkReset, err = a.condition(a.resetOperator, floatValue, a.value)
	}
	if err != nil {
		return false, false, err
	}
	return checkAlarm, checkReset, err
}
func (a *alarms) checkJson(dataMap map[string]interface{}, data any) (float64, string, error) {
	var floatValue float64
	var strValue string
	var err error
	// check if alarm tag is a json and convert to string or float
	if a.json != "" {
		floatValue, strValue, err = extractValueFromPath(dataMap, a.json)
		if err != nil {
			log.Println("error extracting value:", err)
			return 0.0, "", err
		}
	} else {
		floatValue, strValue, err = ParseValue(data)
	}
	return floatValue, strValue, err
}
func (a *alarms) createContent(floatValue float64, strValue string) (string, interface{}) {
	var msgContent string
	if a.addValue {
		if a.stringValue != "" {
			msgContent = fmt.Sprintf("%s%s%s", a.alarmText, ", value: ", strValue)
		} else {
			msgContent = fmt.Sprintf("%s%s%s", a.alarmText, ", value: ", strconv.FormatFloat(floatValue, 'f', -1, 64))
		}
	} else {
		msgContent = a.alarmText
	}
	var msgValue interface{}
	if strValue != "" {
		msgValue = strValue
	} else {
		msgValue = floatValue
	}
	return msgContent, msgValue
}
func init() {
	err := service.RegisterProcessor(
		"alarm",
		alarmConf,
		func(conf *service.ParsedConfig, mgr *service.Resources) (m service.Processor, err error) {
			m, err = newAlarmProcessor(conf, mgr)
			return
		})
	if err != nil {
		log.Println("error")
		panic(err)
	}
}
func newAlarmProcessor(conf *service.ParsedConfig, mgr *service.Resources) (*alarms, error) {
	json, err := conf.FieldString("json")
	if err != nil {
		return nil, err
	}
	value, err := conf.FieldFloat("value")
	if err != nil {
		return nil, err
	}
	stringValue, err := conf.FieldString("stringValue")
	if err != nil {
		return nil, err
	}
	operator, err := conf.FieldString("operator")
	if err != nil {
		return nil, err
	}
	reset, err := conf.FieldFloat("reset")
	if err != nil {
		return nil, err
	}
	resetOperator, err := conf.FieldString("resetOperator")
	if err != nil {
		return nil, err
	}
	filterTimeString, err := conf.FieldString("filterTime")
	if err != nil {
		return nil, err
	}
	filterTime, err := convertToTime(filterTimeString)
	if err != nil {
		return nil, err
	}
	sendIntervalString, err := conf.FieldString("sendInterval")
	if err != nil {
		return nil, err
	}
	sendInterval, err := convertToTime(sendIntervalString)
	if err != nil {
		return nil, err
	}
	alarmText, err := conf.FieldString("alarmText")
	if err != nil {
		return nil, err
	}
	alarmObject, err := conf.FieldString("alarmObject")
	if err != nil {
		return nil, err
	}
	addToJson, err := conf.FieldBool("addToJson")
	if err != nil {
		return nil, err
	}
	alarmJsonStruct, err := conf.FieldString("alarmJsonStruct")
	if err != nil {
		return nil, err
	}
	sendAlarmOnly, err := conf.FieldBool("sendAlarmOnly")
	if err != nil {
		return nil, err
	}
	addValue, err := conf.FieldBool("addValue")
	if err != nil {
		return nil, err
	}
	cleanMsg, err := conf.FieldBool("cleanMsg")
	if err != nil {
		return nil, err
	}
	addMeta, err := conf.FieldBool("addMeta")
	if err != nil {
		return nil, err
	}
	a := &alarms{
		json:            json,
		value:           value,
		stringValue:     stringValue,
		operator:        operator,
		reset:           reset,
		resetOperator:   resetOperator,
		filterTime:      filterTime,
		sendInterval:    sendInterval,
		alarmText:       alarmText,
		alarmObject:     alarmObject,
		addToJson:       addToJson,
		alarmJsonStruct: alarmJsonStruct,
		sendAlarmOnly:   sendAlarmOnly,
		addValue:        addValue,
		cleanMsg:        cleanMsg,
		addMeta:         addMeta,
		stopTickerChan:  make(chan struct{}),
	}
	if a.filterTime > 0 || a.sendInterval > 0 {
		go a.startTicker()
	}
	return a, nil
}
func (a *alarms) Process(ctx context.Context, msg *service.Message) (service.MessageBatch, error) {
	var msgCopy *service.Message
	_, ping := msg.MetaGetMut("ping") // check if generate input is used with the correct meta

	if ping && !a.timerTriggered {
		return nil, nil // block ping messages
	} else if ping && a.timerTriggered {
		msg = a.savedMsg
	} else {
		msgCopy = msg // copy original message
		a.savedMsg = msg
	}

	// Parse incoming message as structured
	data, err := msg.AsStructured()
	if err != nil {
		log.Println("failed to parse message", data)
		return nil, fmt.Errorf("failed to parse message: %v", err)
	}

	// Convert data to map to update it
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		dataMap = make(map[string]interface{})
	}
	//  check in we use json input
	floatValue, strValue, err := a.checkJson(dataMap, data)
	if err != nil {
		return nil, fmt.Errorf("failed to check json: %v", err)
	}
	// check alarm
	checkAlarm, checkReset, err := a.alarmCheck(floatValue, strValue)
	if err != nil {
		return nil, fmt.Errorf("failed to check alarm: %v", err)
	}
	// if alarm condition is met check if it should send if it is not triggered
	if checkAlarm {
		if a.filterTime == 0 {
			if !a.trigger {
				a.send = true
				a.trigger = true
				a.alarmTriggered = true
			}
		} else {
			a.alarmTriggered = true
			if a.startTime.IsZero() {
				// Condition is true for the first time, start the timer
				a.startTime = time.Now()
			}
		}
	}
	// if reset, reset triggers etc.
	if checkReset {
		a.trigger = false
		a.startTime = time.Time{}
		a.send = false
		a.timerTriggered = false
		a.alarmTriggered = false
		a.lastTriggerTime = time.Time{}
	}

	msgContent, msgValue := a.createContent(floatValue, strValue) // create new content for message
	newMsg := service.NewMessage(nil)
	if a.send {
		newMsg = a.createNewMsg(msgContent, msgValue, dataMap, msg) // create a new message
		a.send = false
		a.timerTriggered = false
	} else {
		if !a.sendAlarmOnly {
			newMsg = msgCopy // send original message
		} else {
			return nil, nil // block all messages
		}
	}
	// Return the new message in a batch
	return service.MessageBatch{newMsg}, nil
}

func (a *alarms) Close(ctx context.Context) error {
	if a.stopTickerChan != nil {
		close(a.stopTickerChan)
		a.stopTickerChan = nil // Prevent future sends on a closed channel
	}
	return nil
}
