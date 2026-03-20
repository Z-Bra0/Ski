class {{FORMULA_CLASS}} < Formula
  desc "{{DESCRIPTION}}"
  homepage "{{HOMEPAGE}}"
  version "{{VERSION_NUMBER}}"
  license "{{LICENSE_ID}}"

  if OS.mac? && Hardware::CPU.arm?
    url "https://github.com/{{REPO_OWNER}}/{{REPO_NAME}}/releases/download/{{VERSION_TAG}}/ski_{{VERSION_NUMBER}}_darwin_arm64.tar.gz"
    sha256 "{{DARWIN_ARM64_SHA}}"
  elsif OS.mac? && Hardware::CPU.intel?
    url "https://github.com/{{REPO_OWNER}}/{{REPO_NAME}}/releases/download/{{VERSION_TAG}}/ski_{{VERSION_NUMBER}}_darwin_amd64.tar.gz"
    sha256 "{{DARWIN_AMD64_SHA}}"
  elsif OS.linux? && Hardware::CPU.arm?
    url "https://github.com/{{REPO_OWNER}}/{{REPO_NAME}}/releases/download/{{VERSION_TAG}}/ski_{{VERSION_NUMBER}}_linux_arm64.tar.gz"
    sha256 "{{LINUX_ARM64_SHA}}"
  else
    url "https://github.com/{{REPO_OWNER}}/{{REPO_NAME}}/releases/download/{{VERSION_TAG}}/ski_{{VERSION_NUMBER}}_linux_amd64.tar.gz"
    sha256 "{{LINUX_AMD64_SHA}}"
  end

  def install
    bin.install "ski"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/ski version").strip
  end
end
