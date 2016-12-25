// Daytime sends an MQTT message at sunrise and sunset, changine the status
// to either daytime or nighttime, respectively.
package daytime

import (
	"fmt"
	"time"

	"github.com/casaplatform/casa"
	"github.com/casaplatform/casa/cmd/casa/environment"
	"github.com/casaplatform/mqtt"
	"github.com/pkg/errors"
	"github.com/spf13/viper"

	"dim13.org/sun"
)

// Register this service with the environment global services list.
func init() {
	environment.RegisterService("daytime", &Daytime{})
}

type Daytime struct {
	casa.Logger
	timer *time.Timer
	casa.MessageClient
	nextStatus string
	timerDone  bool
	lat        float64
	lon        float64
	offset     time.Duration
}

func (d *Daytime) Start(config *viper.Viper) error {
	var userString string
	if config.IsSet("MQTT.User") {
		userString = config.GetString("MQTT.User") + ":" +
			config.GetString("MQTT.Pass") + "@"
	}

	client, err := mqtt.NewClient("tcp://" + userString + "127.0.0.1:1883")
	if err != nil {
		return err
	}

	d.MessageClient = client
	client.Handle(func(msg *casa.Message, err error) {
		if err != nil {
			d.Log("ERROR: " + err.Error())
			return
		}
	})

	// Default to 1 hour ofset
	d.offset = 1 * time.Hour
	if config.IsSet("offset") {
		d.offset = time.Duration(config.GetInt64("offset")) * time.Minute
	}

	d.lat = config.GetFloat64("lat")
	d.lon = config.GetFloat64("lon")

	nextStatus, nextTime, err := d.getNext()
	if err != nil {
		return err
	}

	var currStatus string
	if nextStatus == "Daytime" {
		currStatus = "Nighttime"
	} else {
		currStatus = "Daytime"
	}

	d.nextStatus = nextStatus

	until := nextTime.Sub(time.Now())

	//fmt.Println(nextStatus, nextTime, until)

	var messages = []casa.Message{
		{"Service/Daytime/Status", []byte(currStatus), true},
		{"Service/Daytime/Next/Status", []byte(d.nextStatus), true},
		{"Service/Daytime/Next/Time", []byte(nextTime.String()), true},
		//{"Service/Daytime/Next/In", []byte(until.String()), true},
	}
	for _, msg := range messages {
		err = d.PublishMessage(msg)
		if err != nil {
			d.Log("Error sending daytime status update:", err)
		}
	}

	return nil
	d.timer = time.AfterFunc(until, func() {
		d.timerDone = true
		err := d.PublishMessage(casa.Message{"Service/Daytime/Status", []byte(d.nextStatus), true})
		if err != nil {
			d.Log("Error sending daytime status update:", err)
		}

		// Reset the timer
		nextStatus, nextTime, err := d.getNext()
		if err != nil {
			d.Log(err)
		}

		d.nextStatus = nextStatus

		until := nextTime.Sub(time.Now())

		d.timer.Reset(until)
		d.timerDone = false

		fmt.Println("Time until:", until)

	})
	return nil
}

func (d *Daytime) getNext() (string, *time.Time, error) {

	sunrise, err := sun.Rise(time.Now(), d.lat, d.lon)
	if err != nil {
		return "", nil, errors.Wrap(err, "Unable to calculate sunrise")
	}

	// Trigger sunrise time an hour late becuase it's still dark out
	sunrise = sunrise.Add(d.offset)

	//fmt.Println("Sunrise:", sunrise)

	sunset, err := sun.Set(time.Now(), d.lat, d.lon)
	if err != nil {
		return "", nil, errors.Wrap(err, "Unable to calculate sunset")
	}
	// Trigger sunset time an hour early becuase it's usually getting dark
	sunset = sunset.Add(-d.offset).Local()

	//fmt.Println("Sunset:", sunset)
	//fmt.Println(time.Now().Local())

	var nextStatus string
	var nextTime *time.Time

	t := time.Now()
	//t, _ := time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", "2016-12-23 13:02:42.923683284 -0700 MST")
	if sunrise.After(t) {
		nextTime = &sunrise
		nextStatus = "Daytime"

	} else if sunset.After(t) {
		nextStatus = "Nighttime"
		nextTime = &sunset
	} else {
		fmt.Println("Daytime doesnt know what's next")
		//fmt.Println(sunset.After(time.Now()))
	}

	//fmt.Println("next:", nextStatus, nextTime)
	return nextStatus, nextTime, nil

}

func (d *Daytime) Stop() error {
	if d.timer != nil && !d.timerDone && !d.timer.Stop() {
		<-d.timer.C
	}
	return d.MessageClient.Close()
}
func (d *Daytime) UseLogger(logger casa.Logger) {
	d.Logger = logger
}
