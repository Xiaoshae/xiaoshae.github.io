# Run with the same locked Liquid version used by Jekyll.
require 'liquid'
require 'yaml'

root = File.expand_path('../..', __dir__)

module BadgeTestFilters
  def relative_url(input)
    "/test-prefix#{input}"
  end
end
Liquid::Template.register_filter(BadgeTestFilters)

config = YAML.load_file(File.join(root, '_config.yml'))
post_defaults = config.fetch('defaults').find { |item| item.dig('scope', 'type') == 'posts' }
raise 'human_authored must default to true' unless post_defaults.dig('values', 'human_authored') == true

template = Liquid::Template.parse(
  File.read(File.join(root, '_includes/human-authorship.html')),
  error_mode: :strict
)

[{}, { 'human_authored' => true }, { 'human_authored' => false }].each do |page|
  output = template.render!({ 'page' => page }, strict_filters: true)
  expected = page['human_authored'] == false ? 'Co-Created-With-AI-Badge-' : 'Written-By-Human-Not-By-AI-Badge-'
  raise "Expected exactly four badges for #{page}" unless output.scan(/<img\b/).length == 4
  raise 'Expected two English badges' unless output.scan(/-en\.svg/).length == 2
  raise 'Expected two Simplified Chinese badges' unless output.scan(/-zh-CN\.svg/).length == 2

  paths = output.scan(%r{src="/test-prefix(/assets/img/[^"]+)"}).flatten
  raise 'Missing base URL support' unless paths.length == 4
  paths.each do |relative|
    raise "Wrong badge family: #{relative}" unless File.basename(relative).start_with?(expected)
    raise 'Badges must remain SVG' unless relative.end_with?('.svg')
    file = File.join(root, relative.delete_prefix('/'))
    raise "Missing badge: #{file}" unless File.file?(file)
    svg = File.read(file)
    raise 'Not an SVG image' unless svg.include?('<svg')
    raise 'Missing MIT notice in adapted badge' if expected.start_with?('Co-') && !svg.include?('Copyright (c) 2025 Made by Human')
  end
  puts "PASS: #{page.empty? ? 'default' : page.inspect} => four #{expected} SVG badges"
end
