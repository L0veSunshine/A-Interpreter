"""矩阵乘法：衡量密集数组下标读取。"""

n = 90

a = []
b = []
c = []
for i in range(n * n):
    a.append((i * 7 + 3) % 13)
    b.append((i * 11 + 5) % 17)
    c.append(0)

for i in range(n):
    row = i * n
    for j in range(n):
        total = 0
        for k in range(n):
            total += a[row + k] * b[k * n + j]
        c[row + j] = total

checksum = 0
for i in range(n * n):
    checksum += c[i]
print(checksum)
