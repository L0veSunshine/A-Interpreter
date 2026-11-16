"""LeetCode 912: Sort an Array —— 自顶向下归并排序。"""


def merge(a, tmp, lo, mid, hi):
    i = lo
    j = mid + 1
    k = lo
    while i <= mid and j <= hi:
        if a[i] <= a[j]:
            tmp[k] = a[i]
            i += 1
        else:
            tmp[k] = a[j]
            j += 1
        k += 1
    while i <= mid:
        tmp[k] = a[i]
        i += 1
        k += 1
    while j <= hi:
        tmp[k] = a[j]
        j += 1
        k += 1
    for p in range(lo, hi + 1):
        a[p] = tmp[p]
    return 0


def msort(a, tmp, lo, hi):
    if lo >= hi:
        return 0
    mid = (lo + hi) // 2
    msort(a, tmp, lo, mid)
    msort(a, tmp, mid + 1, hi)
    merge(a, tmp, lo, mid, hi)
    return 0


n = 30000
a = []
tmp = []
seed = 987654321
for _ in range(n):
    seed = (seed * 1103515245 + 12345) % 2147483648
    a.append(seed % 100000)
    tmp.append(0)

msort(a, tmp, 0, n - 1)

sorted_flag = 1
total = 0
for i in range(n):
    if i > 0 and a[i - 1] > a[i]:
        sorted_flag = 0
    total += a[i]

print([sorted_flag, total])
