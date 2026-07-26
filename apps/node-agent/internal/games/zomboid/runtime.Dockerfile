# runtime-steam — base image ของ instance ที่ติดตั้ง artifact ผ่าน SteamCMD
#
# ไฟล์นี้ถูก embed เข้าไปใน binary ของ node-agent (ดู runtime.go) — agent build image นี้เอง
# ครั้งแรกที่ต้องใช้แล้ว cache ไว้ให้ instance อื่นบน node เดียวกัน (ไม่มี image สำเร็จรูป
# จาก upstream ให้ pull แบบ JVM). `make runtime-images` build ตัวเดียวกันนี้ล่วงหน้าได้
#
# image นี้ไม่รู้จักเกมใดเป็นพิเศษ — มีแค่ SteamCMD + lib ที่ binary ของเกมบน Linux ต้องใช้
# ตัว artifact และ launch script อยู่ใน bind mount /data (node-agent เป็นคน provision)
# container ถูกสร้างโดย node-agent ด้วย: cap-drop=ALL, no-new-privileges,
# user 1000:1000, memory limit, network game-manager-servers เท่านั้น
FROM debian:bookworm-slim

LABEL project="game-manager"

# steamcmd เป็น binary 32-bit เสมอ (Valve ไม่เคย build 64-bit) จึงต้องเปิด i386 ทั้งที่
# ตัว game server เป็น 64-bit — ขาด lib32gcc-s1 แล้ว steamcmd จะตายทันทีที่รัน
RUN dpkg --add-architecture i386 \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        lib32gcc-s1 \
        lib32stdc++6 \
        libstdc++6 \
        libcurl4 \
        tzdata \
    && rm -rf /var/lib/apt/lists/*

# SteamCMD จาก Valve โดยตรงเท่านั้น (ห้ามใช้ image สำเร็จรูปของคนอื่น)
# ตัว bootstrap อัปเดตตัวเองตอนรันครั้งแรก จึงต้องเป็นของ uid ที่ container รัน
RUN mkdir -p /steamcmd \
    && curl -fsSL https://media.steampowered.com/client/installer/steamcmd_linux.tar.gz \
       | tar -xz -C /steamcmd \
    && chown -R 1000:1000 /steamcmd

# HOME ชี้เข้า /data เพราะทั้ง SteamCMD และตัวเกมเขียน state ลง $HOME
ENV HOME=/data
WORKDIR /data

STOPSIGNAL SIGTERM

# launch.sh ถูก generate ตอน provision — ต้อง exec runtime เป็น process สุดท้ายเสมอ
CMD ["/bin/sh", "/data/.gamemanager/launch.sh"]
