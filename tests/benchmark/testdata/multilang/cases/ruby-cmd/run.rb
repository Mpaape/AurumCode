def archive(name)
  system("tar czf out.tgz #{name}")
end
