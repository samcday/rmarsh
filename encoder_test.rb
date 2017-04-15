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

print Marshal.load($stdin).inspect
