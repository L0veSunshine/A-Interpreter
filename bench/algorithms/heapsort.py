"""LeetCode 912: Sort an Array —— 原地堆排序，衡量下沉（sift-down）与数组交换。"""

n = 200000


def sift_down(a, start, end):
    root = start
    while 2 * root + 1 <= end:
        child = 2 * root + 1
        swap = root
        if a[swap] < a[child]:
            swap = child
        if child + 1 <= end and a[swap] < a[child + 1]:
            swap = child + 1
        if swap == root:
            return
        a[root], a[swap] = a[swap], a[root]
        root = swap


def heapsort(a):
    size = len(a)
    for s in range((size - 2) // 2, -1, -1):
        sift_down(a, s, size - 1)
    for end in range(size - 1, 0, -1):
        a[0], a[end] = a[end], a[0]
        sift_down(a, 0, end - 1)


a = []
seed = 12345
for _ in range(n):
    seed = (seed * 1103515245 + 12345) % 2147483648
    a.append(seed % 100000)

heapsort(a)

ok = 1
total = 0
for i in range(n):
    if i > 0 and a[i - 1] > a[i]:
        ok = 0
    total += a[i]
print([ok, total])
