class NilClass
  def to_s
    'nil'
  end
end
class Symbol
  def to_s
    ':'+id2name
  end
end
class Class
  def to_s
    "Class<#{name}>"
  end
end
class Module
  def to_s
    "Module<#{name}>"
  end
end

print Marshal.load($stdin)