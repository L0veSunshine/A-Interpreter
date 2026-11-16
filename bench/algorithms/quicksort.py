"""LeetCode 912: Sort an Array —— 原地快速排序（Lomuto 分区）。

数据由固定 LCG 生成，保证两种语言拿到完全相同的输入。
"""


def partition(a, lo, hi):
    pivot = a[hi]
    i = lo
    for j in range(lo, hi):
        if a[j] <= pivot:
            a[i], a[j] = a[j], a[i]
            i += 1
    a[i], a[hi] = a[hi], a[i]
    return i


def qsort(a, lo, hi):
    if lo >= hi:
        return 0
    p = partition(a, lo, hi)
    qsort(a, lo, p - 1)
    qsort(a, p + 1, hi)
    return 0


n = 30000
a = []
seed = 12345
for _ in range(n):
    seed = (seed * 1103515245 + 12345) % 2147483648
    a.append(seed % 100000)

qsort(a, 0, n - 1)

sorted_flag = 1
total = 0
for i in range(n):
    if i > 0 and a[i - 1] > a[i]:
        sorted_flag = 0
    total += a[i]

print([sorted_flag, total])
