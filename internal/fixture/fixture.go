// Package fixture holds the values o3s tests use for examples, one value for each part a fixture plays, so every
// test reads the same and a value changes in one place.
//
// Names follow the domains RFC 2606 reserves for examples and addresses the ranges RFC 5737 reserves for
// documentation. A test keeps a literal where that value is itself what it checks, such as the order of versions, a
// malformed address, or a real release the stack runs.
package fixture

import (
	"strconv"
	"time"
)

// Hosts the allowlist names.
const (
	// Host is the host a test allows.
	Host = "example.com"
	// Subdomain is a name below Host.
	Subdomain = "api.example.com"
	// OtherHost is a second host, apart from Host.
	OtherHost = "example.org"
	// OtherWildcard covers every name below OtherHost.
	OtherWildcard = "*.example.org"
	// ThirdHost is a third host, apart from the other two.
	ThirdHost = "example.net"
	// RefusedName is a name the gateway refuses.
	RefusedName = "blocked.example"
	// AbsentName is a name nothing knows.
	AbsentName = "absent.example"
)

// Machines a home runs on.
const (
	// SSHHost is the name a client reaches the machine by over SSH.
	SSHHost = "devbox.example"
)

// Addresses the allowlist names.
const (
	// Address is a single address.
	Address = "192.0.2.1"
	// Range is a range of addresses.
	Range = "198.51.100.0/24"
)

// Ports a host opens.
const (
	// Port is the port a host opens by default.
	Port = 443
	// OtherPort is a second port.
	OtherPort = 22
	// BadPort lies past the last port there is.
	BadPort = 70000
)

// Secrets the proxy injects.
const (
	// Secret is a secret a host carries.
	Secret = "EXAMPLE_TOKEN"
	// OtherSecret is a second secret.
	OtherSecret = "OTHER_TOKEN"
	// NewSecret is a secret the home has yet to know.
	NewSecret = "NEW_TOKEN"
	// SecretValue is the value a secret holds.
	SecretValue = "s3cr3t"
)

// Stores and the features they offer.
const (
	// Store is a store the tests reach.
	Store = "registry.example.com/features"
	// AbsentStore is a store out of reach.
	AbsentStore = "registry.example.org/features"
	// Feature is a feature that needs hosts, a secret and options.
	Feature = "tool"
	// OtherFeature is a plain second feature.
	OtherFeature = "other"
	// ThirdFeature is a third feature, which shares an option with Feature.
	ThirdFeature = "go"
)

// Repositories a project clones.
const (
	// Repo is a repository named by its owner, on the default host.
	Repo = "example/app"
	// NestedRepo is a repository on its own host, with groups above it.
	NestedRepo = "example.com/group/sub/app"
)

// Versions of o3s and of features.
const (
	// Version is the version running or installed now.
	Version = "1.0.0"
	// NewerVersion is the version an update moves to, a major above Version.
	NewerVersion = "2.0.0"
	// Candidate is the first release candidate of NewerVersion.
	Candidate = "2.0.0-rc.1"
	// NextCandidate is the release candidate after Candidate.
	NextCandidate = "2.0.0-rc.2"
	// FurtherCandidate is a release candidate beyond NewerVersion, which only the candidates' channel takes.
	FurtherCandidate = "3.0.0-rc.1"
	// DevBuild is a build that names no release.
	DevBuild = "1.0.0-SNAPSHOT-abc1234"
)

// Tags a feature carries.
const (
	// Major is the tag of Version's major.
	Major = "1"
	// NewerMajor is the tag of NewerVersion's major.
	NewerMajor = "2"
	// MinorTag pins more than a major.
	MinorTag = "1.0"
	// FloatingTag follows every release.
	FloatingTag = "latest"
)

// Moments, in UTC with whole seconds.
const (
	// NowText is the moment a test looks from, Go's own reference time, which names no day of its own.
	NowText = "2006-01-02T15:04:05Z"
	// InAnHourText is an hour after NowText.
	InAnHourText = "2006-01-02T16:04:05Z"
	// InTwoHoursText is two hours after NowText.
	InTwoHoursText = "2006-01-02T17:04:05Z"
	// LongAgoText lies before any moment a test runs at, for a test that looks from the real clock.
	LongAgoText = "2000-01-01T00:00:00Z"
	// FarAheadText lies after any moment a test runs at, as far ahead as the kernel's clock rules reach.
	FarAheadText = "2037-12-31T23:59:59Z"
)

// The moments as times.
var (
	// Now is NowText as a time.
	Now = at(NowText)
	// InAnHour is InAnHourText as a time.
	InAnHour = at(InAnHourText)
	// InTwoHours is InTwoHoursText as a time.
	InTwoHours = at(InTwoHoursText)
	// LongAgo is LongAgoText as a time.
	LongAgo = at(LongAgoText)
	// FarAhead is FarAheadText as a time.
	FarAhead = at(FarAheadText)
)

// at reads a moment the set holds, every one of which reads.
func at(text string) time.Time {
	t, _ := time.Parse(time.RFC3339, text)
	return t
}

// Env hands every value to a test script by a name of its own, the way the script reads it.
func Env() map[string]string {
	return map[string]string{
		"HOST":              Host,
		"SUBDOMAIN":         Subdomain,
		"OTHER_HOST":        OtherHost,
		"OTHER_WILDCARD":    OtherWildcard,
		"THIRD_HOST":        ThirdHost,
		"REFUSED_NAME":      RefusedName,
		"ABSENT_NAME":       AbsentName,
		"SSH_HOST":          SSHHost,
		"ADDRESS":           Address,
		"RANGE":             Range,
		"PORT":              strconv.Itoa(Port),
		"OTHER_PORT":        strconv.Itoa(OtherPort),
		"BAD_PORT":          strconv.Itoa(BadPort),
		"SECRET":            Secret,
		"OTHER_SECRET":      OtherSecret,
		"NEW_SECRET":        NewSecret,
		"SECRET_VALUE":      SecretValue,
		"STORE":             Store,
		"ABSENT_STORE":      AbsentStore,
		"FEATURE":           Feature,
		"OTHER_FEATURE":     OtherFeature,
		"THIRD_FEATURE":     ThirdFeature,
		"REPO":              Repo,
		"NESTED_REPO":       NestedRepo,
		"VERSION":           Version,
		"NEWER_VERSION":     NewerVersion,
		"CANDIDATE":         Candidate,
		"NEXT_CANDIDATE":    NextCandidate,
		"FURTHER_CANDIDATE": FurtherCandidate,
		"DEV_BUILD":         DevBuild,
		"MAJOR":             Major,
		"NEWER_MAJOR":       NewerMajor,
		"MINOR_TAG":         MinorTag,
		"FLOATING_TAG":      FloatingTag,
		"NOW":               NowText,
		"IN_AN_HOUR":        InAnHourText,
		"IN_TWO_HOURS":      InTwoHoursText,
		"LONG_AGO":          LongAgoText,
		"FAR_AHEAD":         FarAheadText,
	}
}
