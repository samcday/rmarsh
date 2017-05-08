class Hash
  # We don't guarantee key ordering on Golang side, so to make things easier
  # we sort keys on this side.
  def inspect
    # If the testing ivar is set on this object, then we print a special case for that.
    if @ivartest
      return "IVarTest<#{@ivartest.inspect}>"
    end

    "{#{keys.sort.map{|k|k.inspect+'=>'+self[k].inspect}.join(', ')}}"
  end
end
class Object
  def inspect
    "\#Object<#{instance_variables.sort.map{|k|k.inspect+'='+instance_variable_get(k).inspect}.join(' ')}>"
  end
end
class Class
  def inspect
    "Class<#{name}>"
  end
end
class Module
  def inspect
    "Module<#{name}>"
  end
end
class UsrMarsh
  def marshal_load data
    @data = data
  end
  def inspect
    "UsrMarsh<#{@data.inspect}>"
  end
end
class UsrDef
  def initialize data
    @data = data
  end
  def self._load data
    UsrDef.new(data)
  end
  def inspect
    "UsrDef<#{@data.inspect}>"
  end
end

TestStruct = Struct.new(:test) do
  def inspect
    "TestStruct<#{test.inspect}>"
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
