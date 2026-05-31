class Devup < Formula
  desc "Local-first remote development CLI for VPS workflows"
  homepage "https://github.com/chnthkksn/homebrew-devup"
  url "https://github.com/chnthkksn/homebrew-devup/archive/refs/tags/v0.6.0.tar.gz"
  sha256 "e992976040ba06c6a73ba113e7d54ae513a0c5d8a33fe39290ade8eb96ab8610"
  license "MIT"

  depends_on "go" => :build
  depends_on "mutagen-io/mutagen/mutagen"

  def install
    system "go", "build", *std_go_args(output: bin/"devup"), "./cmd/devup"
  end

  test do
    assert_match "Usage:", shell_output("#{bin}/devup 2>&1", 2)
  end
end
