package lightware

import (
	"crypto/tls"
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/plugins/inputs"
	"github.com/segmentio/go-snakecase"
)

func init() {
	inputs.Add("lightware", func() telegraf.Input {
		return &Lightware{}
	})
}

type FieldType string

const (
	FieldTypeInteger FieldType = "integer"
	FieldTypeFloat   FieldType = "float"
	FieldTypeBoolean FieldType = "boolean"
	FieldTypeString  FieldType = "string"
)

const Measurement = "lightware"

//go:embed sample.conf
var sampleConfig string

type Device struct {
	// URL of the device to query.
	Url string `toml:"url"`

	// Tags to add to the metrics.
	Tags map[string]string `toml:"tags"`
}

type Path struct {
	// Path to the value.
	Path string

	// Field name to store the value.
	//
	// Default: snake_case(path)
	Field string

	// Type of the value.
	//
	// "integer", "float", "boolean", "string"
	//
	// Default: "string"
	Type string // Default: "string"
}

type Lightware struct {
	wg sync.WaitGroup

	// Devices to query.
	Devices []Device `toml:"devices"`

	// Paths to fetch.
	Paths []*Path `toml:"paths"`

	// Timeout for HTTP requests in seconds.
	Timeout float64 `toml:"timeout"`

	Log telegraf.Logger `toml:"-"`
}

func (l *Lightware) SampleConfig() string {
	return sampleConfig
}

func (l *Lightware) Description() string {
	return "Read metrics from Lightware devices"
}

func (l *Lightware) defaults() {
	if l.Timeout == 0 {
		// How do we set a sensible default?
		// I don't think there is any point setting it longer than the poll interval but as far as I can tell, telegraf doesn't expose it to plugins.
		l.Timeout = 5
	}

	for _, path := range l.Paths {
		if path.Field == "" {
			path.Field = snakecase.Snakecase(path.Path)
		}

		if path.Type == "" {
			path.Type = "string"
		}
	}
}

func (l *Lightware) Gather(acc telegraf.Accumulator) error {
	l.defaults()

	for _, device := range l.Devices {
		l.wg.Add(1)
		go func(device Device) {
			defer l.wg.Done()
			l.gather(device, acc)
		}(device)
	}
	l.wg.Wait()

	return nil
}

var boolRE = regexp.MustCompile(`^(?i)(?:true|1|ok|occupied)$`)

func (l *Lightware) gather(device Device, acc telegraf.Accumulator) {
	u, err := url.Parse(device.Url)
	if err != nil {
		l.Log.Errorf("lightware %q parse URL: %s", device.Url, err)
		return
	}

	tags := map[string]string{
		"host": u.Hostname(),
	}
	for k, v := range device.Tags {
		tags[k] = v
	}

	if _, ok := tags["product"]; !ok {
		u.Path = "/api/ProductName"
		if product, err := get(u); err == nil {
			tags["product"] = string(product)
		} else {
			l.Log.Errorf("lightware %q product: %s", u.String(), err)
			acc.AddFields(Measurement, map[string]any{"result_code": int64(1)}, tags)
			return
		}
	}

	if _, ok := tags["mac"]; !ok {
		u.Path = "/api/V1/MANAGEMENT/UID/MACADDRESS/Main"
		if mac, err := get(u); err == nil {
			tags["mac"] = string(mac) // Naming? Ethernet 1 (Main) is generally used for control/management.
		} else {
			l.Log.Errorf("lightware %q mac: %s", u.String(), err)
			acc.AddFields(Measurement, map[string]any{"result_code": int64(1)}, tags)
			return
		}
	}

	if _, ok := tags["label"]; !ok {
		u.Path = "/api/V1/MANAGEMENT/LABEL/DeviceLabel"
		if label, err := get(u); err == nil {
			tags["label"] = string(label)
		} else {
			l.Log.Errorf("lightware %q label: %s", u.String(), err)
			acc.AddFields(Measurement, map[string]any{"result_code": int64(1)}, tags)
			return
		}
	}

	// Should I gather paths concurrently? I don't know how constrained the devices are.
	fields := map[string]any{
		"result_code": int64(0),
	}

	for _, path := range l.Paths {
		// Ensure both /api/V1/... and /V1/... paths work as it's not obvious you need
		// to include /api/ in the URL if you are looking at the 'AVDANCED' view on
		// the device.
		u.Path = filepath.Join("/api/", strings.TrimPrefix(path.Path, "/api"))

		data, err := get(u)
		if err != nil {
			l.Log.Errorf("lightware %q get: %s", u.String(), err)
			// Some paths are only available on certain models so ignore fetching errors
			// in the result_code and just log them.
			// fields["result_code"] = int64(1)
			continue
		}

		value, err := parse(string(data), FieldType(path.Type))
		if err != nil {
			l.Log.Errorf("lightware %q parse %s: %s", u.String(), path.Type, err)
			fields["result_code"] = int64(1)
			continue
		}

		fields[path.Field] = value
	}

	acc.AddFields(Measurement, fields, tags)
}

var client = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // Ignore self-signed certificates
		},
	},
}

func get(u *url.URL) ([]byte, error) {
	request, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("request: %s", err)
	}
	if password, ok := u.User.Password(); ok {
		request.SetBasicAuth(u.User.Username(), password)
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("response: %s", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status: %d", response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %s", err)
	}

	return body, nil
}

func parse(s string, ft FieldType) (any, error) {
	switch ft {
	case FieldTypeInteger:
		return strconv.ParseInt(s, 10, 64)
	case FieldTypeFloat:
		return strconv.ParseFloat(s, 64)
	case FieldTypeBoolean:
		return boolRE.MatchString(s), nil
	case FieldTypeString:
		return s, nil
	default:
		return nil, fmt.Errorf("unknown type: %s", ft)
	}
}
