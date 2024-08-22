# Benthos alarm processor

## Version 1.0
First release. 

TODO:
* Nothing


## Get started
The basic settings needed for an alarm is:
```yaml
pipeline:
  processors:
    - alarm:
        value: 43.0
        operator: ">"
        reset: 5.0 
        resetOperator: "<"
```
When the conditions for the alarm is met it will send 1 message and block until reset

**Default alarm message:**  
```json   
{
  alarm: true,
  msg: "Alarm",    
  value: 123,    
  conditions: "Alarm when value is greater than 43.0 and reset when  value is smaller than 5.0" 
}  
```
- **value**  
This is the value that triggers the alarm.
For example if using a mqtt input that sends a temperature value
and you want the alarm to trigger at the value 50, you choose value: 50.0
- **operator**  
The operator is how the value gets compared.
For example if you want the alarm to trigger 
if the value is greater than 50 you set it to operator: ">"
Allowed values are "<", ">" "<=", ">=", "=", "!="
Smaller than, Greater than, Smaller or equal, Greater or equal, equal, not equal 
- **reset**  
The value when the alarm should reset.
- **resetOperator**  
Same as the operator but for resetting the alarm.

### Additional setings  
**json:**  
Json object for the alarm in case of json-data. e.x. if the data is: 
```json
{imaginary_value: 312, not_a_number: "grashopper", not_prime_number: 4}
```  
and you want to watch "not_prime_number" use:  
**json:** "not_prime_number"    
for multilevel json like:
```json
{forest: 123, my_neighbors_bedroom:133, treadmills:{sauna1:313}}
```  
and you want to watch "sauna1" use:  
**json:** "treadmills.sauna1"  
The processor will extract the value from the choosen json and compare with the alarm value.  

**stringValue:**  
String to compare for trigger the alarm when using a specific string and not a float value  

**filterTime:**  
Time the condition needs to be true before sending the alarm, if the reset condition is met during this time the filter will be reset and no alarm will be sent. Use h, m and s. "4h12m12s", or "3s"   
**!!! Requires broker input with generate message !!!**  

**sendInterval:**  
Interval time for resending the alarm. for example "1h30m" will send the alarm every 1 and a half hour until reset. Use h, m and s. "4h12m12s", or "3s"  
**!!! Requires broker input with generate message !!!**  

**alarmText:**  
Alarm text for the message  

**alarmObject:**  
Name of the alarm object for the output message e.x is the alarm object is "tempAlarm" the message will be:
```json
{"tempAlarm": true, "msg": "Alarm", "value": 123}  
```
**addToJson:**  
Add the alarm message to the input json structure  

**alarmJsonStruct:**  
Specific json output struct to use if the alarm should be added to an existing json. 
e.x:  
"firstLevel.secondLevel.hereIsMyData"

**sendAlarmOnly:**  
Blocks all messages except the alarm message  

**addValue:**  
Adds the value to the alarm text. e.x: "value: 123" will be added the choosen alarm text  

**cleanMsg:** 
Discards the original message and creates a new json-message containing  
send, msg and value. e.x:  
```json
{"send": true, "msg": "alarm text", "value": 123}
```

**addMeta:**  
Add existing metadata to the message

# NOTE!
When using the filterTime and/or the sendInterval a broker input with generate must be used with the settings   
```yaml
interval: '@every 1s'
mapping: meta ping = true
```
example input:  
```yaml
input:
  broker:
    inputs:
      - mqtt:
          urls: [tcp://192.168.0.1:1883]
          client_id: "alarm"
          connect_timeout: 30s
          topics: [test/new/prostetic_brain]
          auto_replay_nacks: true
      - generate:
          interval: '@every 1s'
          mapping: meta ping = true
```

### Default values
These are the default settings the processor will use it nothing else is specified.
|Name   | Default value|
|:----------------|------:|
**value:** | 100.0  
**json:**  |""  
**stringValue:** | ""  
**operator:** | ">"  
**reset:** | 0.0  
**resetOperator:** | "<"  
**filterTime:** | "0s"  
**sendInterval:** | "0s"  
**alarmText:** | "Alarm"  
**alarmObject:** | "alarm"  
**addToJson:** | false  
**alarmJsonStruct:** | ""  
**sendAlarmOnly:** | true  
**addValue:** | false  
**cleanMsg:** | true  
**addMeta:** | false  

## Example Configuration
```yaml
input:
  broker:
    inputs:
      - mqtt:
          urls: [tcp://192.168.3.22:1883]
          client_id: "alarm"
          connect_timeout: 30s
          topics: [test/new/prostetic_brain]
          auto_replay_nacks: true
      - generate:
          interval: '@every 1s'
          mapping: meta ping = true
pipeline:
  processors:
    - alarm:
        json: "eldorado.onion_soup"                
        value: 50.0                                
        stringValue: ""                             
        operator: ">"                               
        reset: 40.0                               
        resetOperator: "<"                         
        filterTime: "5s"                           
        sendInterval: "1h12m5s"                           
        alarmText: "Oh, no. My kitchen exploded"    
        alarmObject: "my_horse_is_broken"           
        addToJson: true                             
        alarmJsonStruct: "eldorado.alarm"           
        sendAlarmOnly: false                        
        addValue: true                             
        cleanMsg: true                              
        addMeta: true                              
  mqtt:
    urls:
      - tcp://192.168.0.1:1883
    topic: 'test/new/prostetic_brain'
    client_id: 'benthos-alarm'
```