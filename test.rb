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

print Marshal.load($stdin)