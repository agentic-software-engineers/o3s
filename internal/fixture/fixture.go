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
	// NewHost and NextHost are two hosts a test allows afresh, apart from every other.
	NewHost  = "new.example.net"
	NextHost = "next.example.net"
)

// What travels through the relay between the cage and the host.
const (
	// Message is what the cage sends to a service on the host.
	Message = "hello from the cage"
	// Reply is what a service on the host sends back.
	Reply = "hello from the host"
	// SocketDir is the folder where the host's services listen for the cage.
	SocketDir = "/run/o3s"
)

// What the gateway records for a host, as the screen shows it.
const (
	// DataMoved is the data a host moved.
	DataMoved = "1.2 GB"
	// LastSeen is when a host last moved data, or a name was last refused.
	LastSeen = "2m ago"
	// Attempts is how often the gateway refused a name.
	Attempts = 3
)

// What a run leaves behind.
const (
	// Duration is how long a host stays open, as the user types it.
	Duration = "90m"
	// LogFile is where a step writes its whole log.
	LogFile = "/var/log/o3s/example.log"
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
	// TokenValue is a secret long enough to be told apart inside base64.
	TokenValue = "tok-0123456789abcdef0123456789abcdef"
	// Marker is the placeholder the cage sends in place of every secret.
	Marker = "o3s-00112233445566778899aabbccddeeff"
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
	// OtherRepo is a second repository, apart from Repo.
	OtherRepo = "example/other"
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

// Options a feature offers, one of each kind the feature spec knows.
const (
	// Toggle is an option that takes true or false.
	Toggle = "debug"
	// Choice is an option that takes one of a closed list of values.
	Choice = "mode"
	// ChoiceValue is the value Choice falls back to.
	ChoiceValue = "strict"
	// OtherChoiceValue is the other value Choice takes.
	OtherChoiceValue = "loose"
	// Free is an option that takes any text, with values it proposes.
	Free = "version"
	// Proposal is a value Free proposes beside FloatingTag.
	Proposal = "lts"
)

// What a store and an update say about the parts they offer.
const (
	// FeatureTitle is the name Feature shows by.
	FeatureTitle = "Tool"
	// FeatureAbout says what Feature does.
	FeatureAbout = "Installs the tool"
	// OtherFeatureAbout says what OtherFeature does.
	OtherFeatureAbout = "Another tool"
	// ToggleAbout says what Toggle does.
	ToggleAbout = "Log every step"
	// ChoiceAbout says what Choice does.
	ChoiceAbout = "How strict the tool is"
	// FreeAbout says what Free does.
	FreeAbout = "The version to install"
	// StackChange is what a newer stack changes.
	StackChange = "1 file, 2 images"
)

// Folders a test works in.
const (
	// HomeDir is the folder a test's home lives in.
	HomeDir = "/home/ubuntu/o3s"
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
		"HOST":                Host,
		"SUBDOMAIN":           Subdomain,
		"OTHER_HOST":          OtherHost,
		"OTHER_WILDCARD":      OtherWildcard,
		"THIRD_HOST":          ThirdHost,
		"REFUSED_NAME":        RefusedName,
		"ABSENT_NAME":         AbsentName,
		"NEW_HOST":            NewHost,
		"NEXT_HOST":           NextHost,
		"MESSAGE":             Message,
		"REPLY":               Reply,
		"SOCKET_DIR":          SocketDir,
		"DATA_MOVED":          DataMoved,
		"LAST_SEEN":           LastSeen,
		"ATTEMPTS":            strconv.Itoa(Attempts),
		"DURATION":            Duration,
		"LOG_FILE":            LogFile,
		"SSH_HOST":            SSHHost,
		"ADDRESS":             Address,
		"RANGE":               Range,
		"PORT":                strconv.Itoa(Port),
		"OTHER_PORT":          strconv.Itoa(OtherPort),
		"BAD_PORT":            strconv.Itoa(BadPort),
		"SECRET":              Secret,
		"OTHER_SECRET":        OtherSecret,
		"NEW_SECRET":          NewSecret,
		"SECRET_VALUE":        SecretValue,
		"TOKEN_VALUE":         TokenValue,
		"MARKER":              Marker,
		"STORE":               Store,
		"ABSENT_STORE":        AbsentStore,
		"FEATURE":             Feature,
		"OTHER_FEATURE":       OtherFeature,
		"THIRD_FEATURE":       ThirdFeature,
		"REPO":                Repo,
		"OTHER_REPO":          OtherRepo,
		"NESTED_REPO":         NestedRepo,
		"VERSION":             Version,
		"NEWER_VERSION":       NewerVersion,
		"CANDIDATE":           Candidate,
		"NEXT_CANDIDATE":      NextCandidate,
		"FURTHER_CANDIDATE":   FurtherCandidate,
		"DEV_BUILD":           DevBuild,
		"MAJOR":               Major,
		"TOGGLE":              Toggle,
		"CHOICE":              Choice,
		"CHOICE_VALUE":        ChoiceValue,
		"OTHER_CHOICE_VALUE":  OtherChoiceValue,
		"FREE":                Free,
		"PROPOSAL":            Proposal,
		"FEATURE_TITLE":       FeatureTitle,
		"FEATURE_ABOUT":       FeatureAbout,
		"OTHER_FEATURE_ABOUT": OtherFeatureAbout,
		"TOGGLE_ABOUT":        ToggleAbout,
		"CHOICE_ABOUT":        ChoiceAbout,
		"FREE_ABOUT":          FreeAbout,
		"STACK_CHANGE":        StackChange,
		"HOME_DIR":            HomeDir,
		"NEWER_MAJOR":         NewerMajor,
		"MINOR_TAG":           MinorTag,
		"FLOATING_TAG":        FloatingTag,
		"NOW":                 NowText,
		"IN_AN_HOUR":          InAnHourText,
		"IN_TWO_HOURS":        InTwoHoursText,
		"LONG_AGO":            LongAgoText,
		"FAR_AHEAD":           FarAheadText,
	}
}
