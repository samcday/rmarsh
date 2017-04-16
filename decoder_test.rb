$stdout.sync = true

begin
  while true
    begin
      print Marshal.dump(eval($stdin.readline))
      print '$$END$$'
    rescue StandardError => e
      puts "ERROR: #{e}"
    end
  end
rescue Errno::EPIPE
  exit 0
end
