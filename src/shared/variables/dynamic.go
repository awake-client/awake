package variables

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strconv"
	"time"
)

// DynamicPrefix marks a variable computed at send time, such as {{$uuid}}.
const DynamicPrefix = "$"

// DynamicVariable describes a value generated each time a request is sent.
type DynamicVariable struct {
	Name        string
	Description string
	generate    func(now time.Time) string
}

// dynamicVariables lists the built-in dynamic variables. Every occurrence in a
// request receives its own value.
var dynamicVariables = []DynamicVariable{
	{Name: "$uuid", Description: "Random UUID v4", generate: func(time.Time) string { return newUUID() }},
	{Name: "$timestamp", Description: "Unix time in seconds", generate: func(now time.Time) string { return strconv.FormatInt(now.Unix(), 10) }},
	{Name: "$timestampMs", Description: "Unix time in milliseconds", generate: func(now time.Time) string { return strconv.FormatInt(now.UnixMilli(), 10) }},
	{Name: "$isoTimestamp", Description: "Current time in ISO 8601, UTC", generate: func(now time.Time) string { return now.UTC().Format(time.RFC3339) }},
	{Name: "$date", Description: "Current date, YYYY-MM-DD", generate: func(now time.Time) string { return now.UTC().Format(time.DateOnly) }},
	{Name: "$randomInt", Description: "Random integer between 0 and 1000", generate: func(time.Time) string { return strconv.FormatInt(randomInt(1001), 10) }},
	{Name: "$randomHex", Description: "Random 16 character hexadecimal string", generate: func(time.Time) string { return randomHex(8) }},
}

// DynamicVariables returns the built-in dynamic variables, for documentation
// and autocompletion.
func DynamicVariables() []DynamicVariable {
	result := make([]DynamicVariable, len(dynamicVariables))
	copy(result, dynamicVariables)
	return result
}

// now is replaced in tests.
var now = time.Now

func resolveDynamic(name string) (string, bool) {
	for _, variable := range dynamicVariables {
		if variable.Name == name {
			return variable.generate(now()), true
		}
	}
	return "", false
}

func newUUID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(fmt.Sprintf("variables: read random bytes: %v", err))
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16])
}

func randomInt(limit int64) int64 {
	value, err := rand.Int(rand.Reader, big.NewInt(limit))
	if err != nil {
		panic(fmt.Sprintf("variables: read random number: %v", err))
	}
	return value.Int64()
}

func randomHex(size int) string {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		panic(fmt.Sprintf("variables: read random bytes: %v", err))
	}
	return fmt.Sprintf("%x", bytes)
}
