# frozen_string_literal: true

require "digest"
require "fileutils"
require "json"

results = ENV.fetch("BENCHMARK_RESULTS")
FileUtils.mkdir_p(results)
files = Dir.glob("_site/**/*", File::FNM_DOTMATCH).select { |path| File.file?(path) }.sort
manifest = files.map do |path|
  "#{Digest::SHA256.file(path).hexdigest}  #{path.delete_prefix('_site/')}"
end.join("\n") + "\n"
File.write(File.join(results, "site.sha256"), manifest)

bytes = lambda do |path|
  output = IO.popen(["du", "-sb", path], &:read)
  abort "du failed for #{path}" unless $?.success?
  output.split.first.to_i
end
prefix = File.join(ENV.fetch("RUNNER_TOOL_CACHE"), "Ruby/4.0.9/x64")
mode = ENV.fetch("BENCHMARK_MODE")
metrics = {
  mode: mode,
  phase: ENV.fetch("BENCHMARK_PHASE"),
  replica: Integer(ENV.fetch("BENCHMARK_REPLICA")),
  run_id: ENV.fetch("GITHUB_RUN_ID"),
  commit: ENV.fetch("GITHUB_SHA"),
  image: ENV["ImageOS"],
  image_version: ENV["ImageVersion"],
  cpu: File.readlines("/proc/cpuinfo").find { |line| line.start_with?("model name") }&.strip,
  setup_ms: (Integer(ENV.fetch("BENCHMARK_END_NS")) - Integer(ENV.fetch("BENCHMARK_START_NS"))) / 1_000_000.0,
  cache_hit: mode == "official" ? "see setup-ruby cache log" : ENV.fetch("BENCHMARK_CACHE_HIT", ""),
  ruby: RUBY_VERSION,
  rubygems: Gem::VERSION,
  bundler: Bundler::VERSION,
  ruby_prefix_bytes: bytes.call(prefix),
  dependency_bytes: bytes.call("vendor/bundle"),
  extra_gem_home_bytes: mode == "official" ? 0 : bytes.call(".ruby-benchmark/gems"),
  site_files: files.length,
  manifest_sha256: Digest::SHA256.hexdigest(manifest)
}
File.write(File.join(results, "metrics.json"), JSON.pretty_generate(metrics) + "\n")
puts JSON.pretty_generate(metrics)
File.open(ENV.fetch("GITHUB_STEP_SUMMARY"), "a") do |summary|
  summary.puts "### #{metrics[:phase]} / #{mode} / #{metrics[:replica]}"
  summary.puts "Setup (including cold cache save): **#{format('%.3f', metrics[:setup_ms] / 1000)} s**"
  summary.puts "Site: #{files.length} files; manifest `#{metrics[:manifest_sha256]}`"
end
