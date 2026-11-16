"""BFS 最短路：400×400 网格（25% 障碍，固定 LCG 生成），衡量队列与二维下标。"""

R = 400
C = 400
seed = 2024
wall = []
for _ in range(R * C):
    seed = (seed * 1103515245 + 12345) % 2147483648
    wall.append(1 if seed % 100 < 25 else 0)
wall[0] = 0
wall[R * C - 1] = 0

dist = [-1] * (R * C)
dist[0] = 0
queue = [0]
head = 0
while head < len(queue):
    cur = queue[head]
    head += 1
    r = cur // C
    c = cur % C
    d = dist[cur] + 1
    if r > 0 and wall[cur - C] == 0 and dist[cur - C] == -1:
        dist[cur - C] = d
        queue.append(cur - C)
    if r < R - 1 and wall[cur + C] == 0 and dist[cur + C] == -1:
        dist[cur + C] = d
        queue.append(cur + C)
    if c > 0 and wall[cur - 1] == 0 and dist[cur - 1] == -1:
        dist[cur - 1] = d
        queue.append(cur - 1)
    if c < C - 1 and wall[cur + 1] == 0 and dist[cur + 1] == -1:
        dist[cur + 1] = d
        queue.append(cur + 1)
print([dist[R * C - 1], len(queue)])
