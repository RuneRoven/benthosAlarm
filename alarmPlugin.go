package benthosAlarm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/redpanda-data/benthos/v4/public/service"
)

type alarms struct {
	value         int
	stringValue   string
	operator      string
	reset         int
	resetOperator string
	filterTime    time.Duration
	alarmText     string
	addValue      bool
	cleanMsg      bool
	startTime     time.Time // to track when the condition first becomes true
	trigger       bool
}

func convertToInt(value interface{}) (int, error) {
	switch v := value.(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case string:
		return strconv.Atoi(v)
	case json.Number:
		// If the value comes in as json.Number, we can try to parse it as an integer
		if iValue, err := v.Int64(); err == nil {
			return int(iValue), nil
		}
		if fValue, err := v.Float64(); err == nil {
			return int(fValue), nil
		}
		return 0, fmt.Errorf("unable to parse json.Number: %v", v)
	default:
		return 0, fmt.Errorf("unexpected value type: %T", v)
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

func (a *alarms) condition(operator string, value, threshold int) (bool, error) {
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

var alarmConf = service.NewConfigSpec().
	Summary("Creates an processor that sends data when conditions are met. Created by Daniel H").
	Description("This processor plugin enables Benthos to send data when specific conditions are met. " +
		"Configure the plugin by specifying the alarm value, reset value, operator and tagname.").
	Field(service.NewIntField("value").Description("Alarm value, default '100'").Default(100)).
	Field(service.NewStringField("stringValue").Description("Alarm value if using string)").Default("")).
	Field(service.NewStringField("operator").Description("allowed operators: <,>,=").Default(">")).
	Field(service.NewIntField("reset").Description("reset value").Default(0)).
	Field(service.NewStringField("resetOperator").Description("reset operator").Default("<")).
	Field(service.NewStringField("filterTime").Description("Time filter before trigger. Default 0s").Default("0s")).
	Field(service.NewStringField("alarmText").Description("Alarm text to be added to the alarm")).
	Field(service.NewBoolField("addValue").Description("Add the current value to the alarm text").Default(false)).
	Field(service.NewBoolField("cleanMsg").Description("Create a new clean msg with alarmtext only").Default(true))

func init() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("setting up processor")
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
	value, err := conf.FieldInt("value")
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
	reset, err := conf.FieldInt("reset")
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
	alarmText, err := conf.FieldString("alarmText")
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
	return &alarms{
		value:         value,
		stringValue:   stringValue,
		operator:      operator,
		reset:         reset,
		resetOperator: resetOperator,
		filterTime:    filterTime,
		alarmText:     alarmText,
		addValue:      addValue,
		cleanMsg:      cleanMsg,
	}, nil
}

func (a *alarms) Process(ctx context.Context, msg *service.Message) (service.MessageBatch, error) {
	msgCopy := msg
	// Step 1: Parse the incoming message
	data, err := msg.AsStructured()
	if err != nil {
		log.Println("failed to parse message", data)
		return nil, fmt.Errorf("failed to parse message: %v", err)
	}
	log.Println("message as structed", data)
	/*	var topic string
		msg.MetaWalk(func(k, v string) error {
			if len(k) >= 5 && k[len(k)-5:] == "topic" {
				topic = v
				return nil
			}
			return nil
		})
		log.Println("message meta", topic)
	*/

	value, err := convertToInt(data)
	// Debug: Log the extracted value
	//log.Println("Extracted value:", value)

	// Convert data to map to update it
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		dataMap = make(map[string]interface{})
		//log.Println("new map created", dataMap)
	}

	if a.stringValue != "" {
		// add some check here to only use the string if != ""
	}
	// Compare the value with the specified limit
	send := false
	checkAlarm, err := a.condition(a.operator, value, a.value)
	if err != nil {
		return nil, err
	}
	checkReset, err := a.condition(a.resetOperator, value, a.value)
	if err != nil {
		return nil, err
	}

	if checkAlarm {
		if a.filterTime == 0 {
			if !a.trigger {
				send = true
				a.trigger = true
			}
		} else {
			if a.startTime.IsZero() {
				// Condition is true for the first time, start the timer
				a.startTime = time.Now()
			} else if time.Since(a.startTime) >= a.filterTime {
				if !a.trigger {
					send = true
					a.trigger = true
				}
			}
		}
	}
	if checkReset {
		a.trigger = false
		a.startTime = time.Time{}
	}
	//log.Println("trigger:", a.trigger)
	// Debug: Log the comparison result
	//log.Println("Comparison result, sendFlag:", send)

	// Create a new message with 'root.send' set to true
	var msgContent string
	if a.addValue {
		msgContent = fmt.Sprintf("%s%s%s", a.alarmText, ", value: ", strconv.Itoa(value))
	} else {
		msgContent = a.alarmText
	}

	newMsg := service.NewMessage(nil)
	if send {
		if a.cleanMsg {
			newMsg = service.NewMessage([]byte{})
			newMsg.SetStructured(map[string]interface{}{
				"send":  send,
				"msg":   msgContent,
				"value": value,
			})
		} else {
			// Add the new fields to the original data
			dataMap["send"] = send
			dataMap["msg"] = msgContent
			dataMap["value"] = value
			// Extract metadata and add to dataMap
			msg.MetaWalk(func(key, value string) error {
				dataMap[key] = value
				return nil
			})
			// Create a new message with the updated data
			newMsg = service.NewMessage(nil)
			newMsg.SetStructured(dataMap)
		}
	} else {
		newMsg = msgCopy
	}
	//log.Println("Modified message:", newMsg)
	// Return the new message in a batch
	return service.MessageBatch{newMsg}, nil

}

func (r *alarms) Close(ctx context.Context) error {
	log.Println("closing processor")
	return nil
}
