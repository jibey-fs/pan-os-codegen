package high_availability_test

import (
	"bytes"
	"encoding/xml"
	"io"
	"sort"
	"strings"
	"testing"

	"github.com/PaloAltoNetworks/pango/device/high_availability"
	"github.com/PaloAltoNetworks/pango/version"
)

const panosVersion = "11.1.0"

// activeActiveTemplateXml is the high-availability node of a real Panorama
// template for a route-based active/active pair. device-priority is an integer
// leaf set to a template variable, which is why it is modelled as a string.
const activeActiveTemplateXml = `
<high-availability>
  <interface>
    <ha1><port>ha1-a</port><ip-address>$ha1-local-ip</ip-address></ha1>
    <ha2><port>ethernet1/19</port></ha2>
    <ha2-backup><port>ethernet1/20</port></ha2-backup>
    <ha3><port>ae1</port></ha3>
    <ha1-backup><port>ha1-b</port><ip-address>$ha1-backup-local-ip</ip-address></ha1-backup>
  </interface>
  <group>
    <group-id>1</group-id>
    <description>A/A route base HA</description>
    <peer-ip>$ha-peer-ha1-ip</peer-ip>
    <peer-ip-backup>$ha-backup-peer-ha1-ip</peer-ip-backup>
    <election-option>
      <device-priority>$ha-device-priority</device-priority>
      <heartbeat-backup>yes</heartbeat-backup>
      <preemptive>yes</preemptive>
    </election-option>
    <state-synchronization><ha2-keep-alive><enabled>yes</enabled><action>split-datapath</action></ha2-keep-alive></state-synchronization>
    <mode><active-active><device-id>0</device-id><session-owner-selection><first-packet/></session-owner-selection></active-active></mode>
  </group>
  <enabled>yes</enabled>
</high-availability>`

// unmodelledSibling is a child of high-availability the spec does not declare.
const unmodelledSibling = `<cluster><enabled>yes</enabled></cluster>`

func ptr[T any](v T) *T { return &v }

func activeActiveTemplateConfig() *high_availability.Config {
	return &high_availability.Config{
		Enabled: ptr(true),
		Interface: &high_availability.Interface{
			Ha1:       &high_availability.InterfaceHa1{Port: ptr("ha1-a"), IpAddress: ptr("$ha1-local-ip")},
			Ha1Backup: &high_availability.InterfaceHa1Backup{Port: ptr("ha1-b"), IpAddress: ptr("$ha1-backup-local-ip")},
			Ha2:       &high_availability.InterfaceHa2{Port: ptr("ethernet1/19")},
			Ha2Backup: &high_availability.InterfaceHa2Backup{Port: ptr("ethernet1/20")},
			Ha3:       &high_availability.InterfaceHa3{Port: ptr("ae1")},
		},
		Group: &high_availability.Group{
			GroupId:      ptr("1"),
			Description:  ptr("A/A route base HA"),
			PeerIp:       ptr("$ha-peer-ha1-ip"),
			PeerIpBackup: ptr("$ha-backup-peer-ha1-ip"),
			ElectionOption: &high_availability.GroupElectionOption{
				DevicePriority:  ptr("$ha-device-priority"),
				HeartbeatBackup: ptr(true),
				Preemptive:      ptr(true),
			},
			StateSynchronization: &high_availability.GroupStateSynchronization{
				Ha2KeepAlive: &high_availability.GroupStateSynchronizationHa2KeepAlive{
					Enabled: ptr(true),
					Action:  ptr("split-datapath"),
				},
			},
			Mode: &high_availability.GroupMode{
				ActiveActive: &high_availability.GroupModeActiveActive{
					DeviceId: ptr("0"),
					SessionOwnerSelection: &high_availability.GroupModeActiveActiveSessionOwnerSelection{
						FirstPacket: &high_availability.GroupModeActiveActiveSessionOwnerSelectionFirstPacket{},
					},
				},
			},
		},
	}
}

// node is an order-insensitive view of an XML element.
type node struct {
	name     string
	text     string
	children []node
}

func (n node) String() string {
	var b strings.Builder
	n.write(&b, 0)
	return b.String()
}

func (n node) write(b *strings.Builder, depth int) {
	b.WriteString(strings.Repeat("  ", depth) + n.name)
	if n.text != "" {
		b.WriteString("=" + n.text)
	}
	b.WriteString("\n")
	for _, c := range n.children {
		c.write(b, depth+1)
	}
}

func canonical(t *testing.T, data []byte) node {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(data))
	var stack []*node
	var root node
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("parsing XML: %v", err)
		}
		switch tok := tok.(type) {
		case xml.StartElement:
			stack = append(stack, &node{name: tok.Name.Local})
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].text += strings.TrimSpace(string(tok))
			}
		case xml.EndElement:
			done := *stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			sort.Slice(done.children, func(i, j int) bool { return done.children[i].String() < done.children[j].String() })
			if len(stack) == 0 {
				root = done
			} else {
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, done)
			}
		}
	}
	return root
}

func marshal(t *testing.T, config *high_availability.Config) []byte {
	t.Helper()
	specifier, _, err := high_availability.Versioning(mustVersion(t))
	if err != nil {
		t.Fatalf("versioning: %v", err)
	}
	element, err := specifier(config)
	if err != nil {
		t.Fatalf("specifier: %v", err)
	}
	data, err := xml.Marshal(element)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}

func unmarshal(t *testing.T, haXml string) *high_availability.Config {
	t.Helper()
	_, normalizer, err := high_availability.Versioning(mustVersion(t))
	if err != nil {
		t.Fatalf("versioning: %v", err)
	}
	if err := xml.Unmarshal([]byte("<result>"+haXml+"</result>"), normalizer); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	configs, err := normalizer.Normalize()
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("expected 1 config, got %d", len(configs))
	}
	return configs[0]
}

func mustVersion(t *testing.T) version.Number {
	t.Helper()
	vn, err := version.New(panosVersion)
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	return vn
}

func TestMarshalMatchesTemplateXml(t *testing.T) {
	got := canonical(t, marshal(t, activeActiveTemplateConfig())).String()
	want := canonical(t, []byte(activeActiveTemplateXml)).String()
	if got != want {
		t.Errorf("marshalled XML mismatch\n--- got\n%s--- want\n%s", got, want)
	}
}

func TestUnmarshalTemplateXmlMatchesConfig(t *testing.T) {
	got := unmarshal(t, activeActiveTemplateXml)
	if !high_availability.SpecMatches(got, activeActiveTemplateConfig()) {
		t.Errorf("unmarshalled config does not match the expected config")
	}
	if len(got.Misc) != 0 || len(got.Group.Misc) != 0 || len(got.Interface.Misc) != 0 {
		t.Errorf("template XML has unmodelled nodes: %v %v %v", got.Misc, got.Group.Misc, got.Interface.Misc)
	}
}

func TestRewritePreservesUnmodelledSibling(t *testing.T) {
	withSibling := strings.Replace(activeActiveTemplateXml, "<enabled>yes</enabled>\n</high-availability>",
		"<enabled>yes</enabled>"+unmodelledSibling+"</high-availability>", 1)
	if !strings.Contains(withSibling, unmodelledSibling) {
		t.Fatalf("fixture does not contain the unmodelled sibling")
	}
	got := canonical(t, marshal(t, unmarshal(t, withSibling))).String()
	want := canonical(t, []byte(withSibling)).String()
	if got != want {
		t.Errorf("rewrite dropped an unmodelled node\n--- got\n%s--- want\n%s", got, want)
	}
}
