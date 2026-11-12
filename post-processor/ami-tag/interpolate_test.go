package ami_tag

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("normalizeData", func() {
	It("copies string-key map entries unchanged", func() {
		input := map[string]interface{}{
			"Name": "my-ami", "Region": "us-east-1", "Count": 42,
		}
		Expect(normalizeData(input)).To(Equal(map[string]interface{}{
			"Name": "my-ami", "Region": "us-east-1", "Count": 42,
		}))
	})

	It("converts interface-key map keys to strings", func() {
		input := map[interface{}]interface{}{
			"Name": "my-ami", "Region": "us-east-1", "Count": 42,
		}
		Expect(normalizeData(input)).To(Equal(map[string]interface{}{
			"Name": "my-ami", "Region": "us-east-1", "Count": 42,
		}))
	})

	It("produces identical output for both map shapes", func() {
		s := map[string]interface{}{"SourceAMI": "ami-1", "NumValue": 99}
		i := map[interface{}]interface{}{"SourceAMI": "ami-1", "NumValue": 99}
		Expect(normalizeData(i)).To(Equal(normalizeData(s)))
	})

	It("recurses nested interface-key maps", func() {
		input := map[interface{}]interface{}{
			"Tags": map[interface{}]interface{}{
				"Name": "web", "Env": "prod",
			},
		}
		Expect(normalizeData(input)).To(Equal(map[string]interface{}{
			"Tags": map[string]interface{}{"Name": "web", "Env": "prod"},
		}))
	})

	It("returns empty map for nil input", func() {
		out := normalizeData(nil)
		Expect(out).NotTo(BeNil())
		Expect(out).To(HaveLen(0))
	})

	It("returns empty map for unsupported type", func() {
		out := normalizeData(42)
		Expect(out).NotTo(BeNil())
		Expect(out).To(HaveLen(0))
	})

	It("converts non-string keys via fmt.Sprint", func() {
		input := map[interface{}]interface{}{42: "num", true: "bool"}
		Expect(normalizeData(input)).To(Equal(map[string]interface{}{
			"42": "num", "true": "bool",
		}))
	})
})

var _ = Describe("interpolateTags", func() {
	var base interpolate.Context

	BeforeEach(func() {
		base = interpolate.Context{}
	})

	It("performs basic substitution", func() {
		tags := map[string]string{
			"Name":   "{{ .SourceAMIName }}",
			"Region": "{{ .BuildRegion }}",
		}
		data := map[string]interface{}{
			"SourceAMIName": "base-ami", "BuildRegion": "us-east-1",
		}
		out, errs := interpolateTags(tags, base, data)
		Expect(errs).To(BeEmpty())
		Expect(out).To(Equal(map[string]string{
			"Name": "base-ami", "Region": "us-east-1",
		}))
	})

	It("handles mixed literal and template text", func() {
		tags := map[string]string{
			"Desc": "Built from {{ .SourceAMI }} in {{ .BuildRegion }}",
		}
		data := map[string]interface{}{
			"SourceAMI": "ami-123", "BuildRegion": "eu-west-1",
		}
		out, errs := interpolateTags(tags, base, data)
		Expect(errs).To(BeEmpty())
		Expect(out["Desc"]).To(Equal("Built from ami-123 in eu-west-1"))
	})

	It("passes through literal values unchanged", func() {
		tags := map[string]string{
			"Env": "production", "Team": "platform", "Version": "1.2.3",
		}
		out, errs := interpolateTags(tags, base, map[string]interface{}{})
		Expect(errs).To(BeEmpty())
		Expect(out).To(Equal(tags))
	})

	Context("error handling", func() {
		It("reports missing key with tag key and variable name", func() {
			tags := map[string]string{"Name": "{{ .NonExistentKey }}"}
			_, errs := interpolateTags(tags, base, map[string]interface{}{})
			Expect(errs).To(HaveLen(1))
			Expect(errs[0].Error()).To(ContainSubstring("Name"))
			Expect(errs[0].Error()).To(ContainSubstring("NonExistentKey"))
		})

		It("collects multiple errors", func() {
			tags := map[string]string{
				"Alpha":   "{{ .MissingA }}",
				"Beta":    "{{ .MissingB }}",
				"Literal": "ok",
			}
			out, errs := interpolateTags(tags, base, map[string]interface{}{})
			Expect(errs).To(HaveLen(2))
			Expect(out).To(HaveKeyWithValue("Literal", "ok"))
		})
	})

	It("emits data values containing {{ verbatim", func() {
		tags := map[string]string{"Name": "{{ .Val }}"}
		data := map[string]interface{}{"Val": "{{ .ShouldNotExpand }}"}
		out, errs := interpolateTags(tags, base, data)
		Expect(errs).To(BeEmpty())
		Expect(out["Name"]).To(Equal("{{ .ShouldNotExpand }}"))
	})

	It("produces deterministic output for sorted keys", func() {
		tags := map[string]string{
			"C": "{{ .X }}-c", "A": "{{ .X }}-a", "B": "{{ .X }}-b",
		}
		data := map[string]interface{}{"X": "v"}
		first, errs := interpolateTags(tags, base, data)
		Expect(errs).To(BeEmpty())
		for i := 0; i < 10; i++ {
			again, errs := interpolateTags(tags, base, data)
			Expect(errs).To(BeEmpty())
			Expect(again).To(Equal(first))
		}
	})

	It("formats non-string values with fmt.Sprint", func() {
		tags := map[string]string{
			"Num": "{{ .NumericValue }}", "Bool": "{{ .BoolValue }}",
		}
		data := map[string]interface{}{"NumericValue": 42, "BoolValue": true}
		out, errs := interpolateTags(tags, base, data)
		Expect(errs).To(BeEmpty())
		Expect(out["Num"]).To(Equal(fmt.Sprint(42)))
		Expect(out["Bool"]).To(Equal(fmt.Sprint(true)))
	})

	It("handles empty tag map", func() {
		out, errs := interpolateTags(
			map[string]string{}, base, map[string]interface{}{})
		Expect(errs).To(BeEmpty())
		Expect(out).To(BeEmpty())
	})

	Context("context preservation", func() {
		empty := map[string]interface{}{}

		It("preserves UserVariables", func() {
			base.UserVariables = map[string]string{"myvar": "user-value"}
			out, errs := interpolateTags(
				map[string]string{"U": `{{ user "myvar" }}`}, base, empty)
			Expect(errs).To(BeEmpty())
			Expect(out["U"]).To(Equal("user-value"))
		})

		It("preserves BuildName", func() {
			base.BuildName = "my-build"
			out, errs := interpolateTags(
				map[string]string{"B": "{{ build_name }}"}, base, empty)
			Expect(errs).To(BeEmpty())
			Expect(out["B"]).To(Equal("my-build"))
		})

		It("preserves TemplatePath", func() {
			base.TemplatePath = "/some/path/template.json"
			out, errs := interpolateTags(
				map[string]string{"T": "{{ template_dir }}"}, base, empty)
			Expect(errs).To(BeEmpty())
			Expect(out["T"]).To(Equal("/some/path"))
		})

		It("preserves custom Funcs", func() {
			base.Funcs = template.FuncMap{
				"shout": func(s string) string { return strings.ToUpper(s) },
			}
			out, errs := interpolateTags(
				map[string]string{"S": `{{ shout "hello" }}`}, base, empty)
			Expect(errs).To(BeEmpty())
			Expect(out["S"]).To(Equal("HELLO"))
		})

		It("does not mutate the base context", func() {
			base.BuildName = "orig"
			_, errs := interpolateTags(
				map[string]string{"N": "{{ .K }}"}, base,
				map[string]interface{}{"K": "v"})
			Expect(errs).To(BeEmpty())
			Expect(base.Data).To(BeNil())
			Expect(base.BuildName).To(Equal("orig"))
		})
	})
})

var _ = Describe("validateRenderedTags", func() {
	Context("key length", func() {
		It("accepts key at exactly 128 runes", func() {
			k := strings.Repeat("k", 128)
			Expect(validateRenderedTags(map[string]string{k: "v"})).To(BeEmpty())
		})

		It("rejects key at 129 runes", func() {
			k := strings.Repeat("k", 129)
			errs := validateRenderedTags(map[string]string{k: "v"})
			Expect(errs).To(HaveLen(1))
			Expect(errs[0].Error()).To(ContainSubstring(k))
			Expect(errs[0].Error()).To(ContainSubstring("129"))
		})
	})

	Context("value length", func() {
		It("accepts value at exactly 256 runes", func() {
			v := strings.Repeat("v", 256)
			Expect(validateRenderedTags(map[string]string{"mykey": v})).To(BeEmpty())
		})

		It("rejects value at 257 runes", func() {
			v := strings.Repeat("v", 257)
			errs := validateRenderedTags(map[string]string{"mykey": v})
			Expect(errs).To(HaveLen(1))
			Expect(errs[0].Error()).To(ContainSubstring("mykey"))
			Expect(errs[0].Error()).To(ContainSubstring("257"))
		})
	})

	It("reports multiple violations", func() {
		longKey := strings.Repeat("k", 129)
		longVal := strings.Repeat("v", 257)
		errs := validateRenderedTags(map[string]string{
			longKey: "ok", "short": longVal,
		})
		Expect(errs).To(HaveLen(2))
	})

	It("accepts all tags within limits", func() {
		Expect(validateRenderedTags(map[string]string{
			"Name": "web", "Environment": "prod", "Version": "1.0",
		})).To(BeEmpty())
	})

	It("counts Unicode runes not bytes", func() {
		Expect(validateRenderedTags(map[string]string{
			strings.Repeat("日", 128): "v",
		})).To(BeEmpty())
		Expect(validateRenderedTags(map[string]string{
			strings.Repeat("日", 129): "v",
		})).To(HaveLen(1))
		Expect(validateRenderedTags(map[string]string{
			"k": strings.Repeat("字", 256),
		})).To(BeEmpty())
		Expect(validateRenderedTags(map[string]string{
			"k": strings.Repeat("字", 257),
		})).To(HaveLen(1))
	})
})
