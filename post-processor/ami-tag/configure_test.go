package ami_tag

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// baseConfigMap returns the minimum config map for a valid
// Configure call, with the given tags merged in.
func baseConfigMap(tags map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"tags":       tags,
		"access_key": "test-key",
		"secret_key": "test-secret",
		"region":     "us-east-1",
	}
}

var _ = Describe("Configure", func() {
	var p *PostProcessor

	BeforeEach(func() {
		p = &PostProcessor{}
	})

	Context("template expression preservation", func() {
		It("preserves single template expression through Configure", func() {
			raw := baseConfigMap(map[string]interface{}{
				"Name": "{{ .SourceAMIName }}",
			})
			Expect(p.Configure(raw)).To(Succeed())
			Expect(p.config.Tags).To(HaveKeyWithValue(
				"Name", "{{ .SourceAMIName }}"))
		})

		It("preserves multiple template expressions", func() {
			raw := baseConfigMap(map[string]interface{}{
				"Name":   "{{ .SourceAMIName }}",
				"Region": "{{ .BuildRegion }}",
				"Mixed":  "built-from-{{ .SourceAMI }}",
			})
			Expect(p.Configure(raw)).To(Succeed())
			Expect(p.config.Tags).To(HaveKeyWithValue(
				"Name", "{{ .SourceAMIName }}"))
			Expect(p.config.Tags).To(HaveKeyWithValue(
				"Region", "{{ .BuildRegion }}"))
			Expect(p.config.Tags).To(HaveKeyWithValue(
				"Mixed", "built-from-{{ .SourceAMI }}"))
		})

		It("preserves AMITags references", func() {
			raw := baseConfigMap(map[string]interface{}{
				"CopiedTag": `{{ .AMITags.Name }}`,
				"Indexed":   `{{ index .AMITags "a/b c" }}`,
			})
			Expect(p.Configure(raw)).To(Succeed())
			Expect(p.config.Tags).To(HaveKeyWithValue(
				"CopiedTag", `{{ .AMITags.Name }}`))
			Expect(p.config.Tags).To(HaveKeyWithValue(
				"Indexed", `{{ index .AMITags "a/b c" }}`))
		})
	})

	It("still interpolates other config fields", func() {
		raw := baseConfigMap(map[string]interface{}{
			"Name": "literal-value",
		})
		// Success means AccessConfig.Prepare ran and validated
		// the access_key/secret_key/region fields.
		Expect(p.Configure(raw)).To(Succeed())
	})

	Context("validation", func() {
		It("rejects aws: prefix", func() {
			raw := baseConfigMap(map[string]interface{}{
				"aws:Reserved": "some-value",
			})
			err := p.Configure(raw)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("aws:"))
		})

		It("rejects more than 50 tags", func() {
			tags := make(map[string]interface{}, 51)
			for i := range 51 {
				key := strings.Repeat("k", 3) +
					string(rune('A'+i/26)) + string(rune('a'+i%26))
				tags[key] = "v"
			}
			err := p.Configure(baseConfigMap(tags))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exceeds"))
		})

		It("skips length check for templated values", func() {
			long := "{{ .SourceAMIName }}" + strings.Repeat("x", 250)
			raw := baseConfigMap(map[string]interface{}{"Name": long})
			Expect(p.Configure(raw)).To(Succeed())
		})

		It("still rejects literal values exceeding length limit", func() {
			raw := baseConfigMap(map[string]interface{}{
				"Name": strings.Repeat("x", 257),
			})
			err := p.Configure(raw)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("maximum length"))
		})

		It("fails on template parse error naming the tag key", func() {
			raw := baseConfigMap(map[string]interface{}{
				"BadTag": "{{ .X",
			})
			err := p.Configure(raw)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("BadTag"),
				ContainSubstring("unclosed action"),
				ContainSubstring("unexpected"),
			))
		})
	})

	It("is safe to call twice without cached state", func() {
		raw := baseConfigMap(map[string]interface{}{
			"Name": "{{ .SourceAMIName }}",
		})
		Expect(p.Configure(raw)).To(Succeed())
		Expect(p.Configure(raw)).To(Succeed())
		Expect(p.config.Tags).To(HaveKeyWithValue(
			"Name", "{{ .SourceAMIName }}"))
	})
})
