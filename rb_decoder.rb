class Hash
  # We don't guarantee key ordering on Golang side, so to make things easier
  # we sort keys on this side.
  def inspect
    "{#{keys.sort.map{|k|k.inspect+'=>'+self[k].to_s}.join(', ')}}"
  end
end
class Object
  def inspect
    "\#Object<#{instance_variables.sort.map{|k|k.inspect+'='+instance_variable_get(k).to_s}.join(' ')}>"
  end
end

$stdout.sync = true

begin
  while true
    begin
      puts Marshal.load($stdin.readline('$$END$$').chomp('$$END$$')).inspect
    rescue StandardError => e
      puts "ERROR: #{e}"
    end
  end
rescue Errno::EPIPE
  exit 0
end
