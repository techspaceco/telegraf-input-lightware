package lightware

import (
	"crypto/tls"
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/iancoleman/strcase"
	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/plugins/inputs"
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

//go:embed sample.conf
var sampleConfig string

type Path struct {
	// Path to the value.
	Path string

	// Field name to store the value.
	//
	// Default: github.com/iancoleman/strcase.ToSnake(path)
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

	// Urls to fetch from.
	Urls []string

	// Paths to fetch.
	Paths []Path

	// Timeout for HTTP requests in seconds.
	Timeout float64

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
		l.Timeout = 5
	}

	for i, path := range l.Paths {
		if path.Field == "" {
			l.Paths[i].Field = strcase.ToSnake(path.Path)
		}
	}
}

func (l *Lightware) Gather(acc telegraf.Accumulator) error {
	l.defaults()

	for _, host := range l.Urls {
		l.wg.Add(1)
		go func(host string) {
			defer l.wg.Done()
			l.gather(host, acc)
		}(host)
	}
	l.wg.Wait()

	return nil
}

func (l *Lightware) gather(host string, acc telegraf.Accumulator) {
	u, err := url.Parse(host)
	if err != nil {
		l.Log.Errorf("lightware parse URL: %s", err)
		return
	}

	tags := map[string]string{
		"host": u.Hostname(),
	}

	u.Path = "/ProductName"
	if product, err := get(u); err == nil {
		tags["product"] = string(product)
	} else {
		l.Log.Errorf("lightware product: %s", err)
		return
	}

	fields := map[string]any{}

	u.Path = "/PackageVersion"
	if version, err := get(u); err == nil {
		fields["package_version"] = string(version)
	} else {
		l.Log.Errorf("lightware version: %s", err)
		return
	}

	// Should I gather paths concurrently? I don't know how constrained the devices are.
	for _, path := range l.Paths {
		u.Path = filepath.Join(u.Path, path.Path)

		data, err := get(u)
		if err != nil {
			l.Log.Errorf("lightware get: %s", err)
			return
		}

		switch FieldType(path.Type) {
		case FieldTypeInteger:
			value, err := strconv.ParseInt(string(data), 10, 64)
			if err != nil {
				l.Log.Errorf("lightware parse integer: %s", err)
				return
			}
			fields[path.Path] = value
		case FieldTypeFloat:
			value, err := strconv.ParseFloat(string(data), 64)
			if err != nil {
				l.Log.Errorf("lightware parse float: %s", err)
				return
			}
			fields[path.Path] = value
		case FieldTypeBoolean:
			value, err := strconv.ParseBool(string(data))
			if err != nil {
				l.Log.Errorf("lightware parse boolean: %s", err)
				return
			}
			fields[path.Path] = value
		case FieldTypeString:
			fields[path.Path] = string(data)
		default:
			l.Log.Errorf("lightware unknown type: %s", path.Type)
			return
		}

	}

	acc.AddFields("lightware", fields, tags)
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
