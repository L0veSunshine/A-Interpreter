"""LeetCode 204: Count Primes —— 埃拉托斯特尼筛法。"""

n = 1000000

flags = []
for _ in range(n + 1):
    flags.append(1)
flags[0] = 0
flags[1] = 0

i = 2
while i * i <= n:
    if flags[i] == 1:
        for j in range(i * i, n + 1, i):
            flags[j] = 0
    i += 1

count = 0
for k in range(n + 1):
    if flags[k] == 1:
        count += 1
print(count)
