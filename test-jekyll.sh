#!/bin/bash
echo "Testing Jekyll build..."

# Create minimal Gemfile
cat > Gemfile <<'GEMS'
source 'https://rubygems.org'
gem 'jekyll', '~> 4.3'
gem 'just-the-docs', '0.8.0'
GEMS

# Try bundle install
if command -v bundle &> /dev/null; then
  bundle install
  bundle exec jekyll build --trace
else
  echo "Bundle not available - skipping"
fi
