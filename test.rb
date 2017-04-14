v = Marshal.load($stdin)
if v.nil?
  print 'nil'
else
  print v
end