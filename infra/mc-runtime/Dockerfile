# mc-runtime — base image สำหรับรัน Minecraft server ทุก instance
# build 4 tag ด้วย make runtime-images:
#   mcpanel/mc-runtime:8   (MC <= 1.16.5)
#   mcpanel/mc-runtime:17  (MC 1.17 - 1.20.4)
#   mcpanel/mc-runtime:21  (MC 1.20.5 - 1.21.x)
#   mcpanel/mc-runtime:25  (MC calendar version 26.x+, velocity, fallback)
# ARG JAVA_VERSION รองรับค่าอื่นได้ถ้า Minecraft ขยับไป Java ใหม่กว่านี้อีก
#
# ตัว jar และ launch script อยู่ใน bind mount /mc (node-agent เป็นคน provision)
# container ถูกสร้างโดย node-agent ด้วย: cap-drop=ALL, no-new-privileges,
# user 1000:1000, memory limit, network mcpanel-servers เท่านั้น
ARG JAVA_VERSION=21
FROM eclipse-temurin:${JAVA_VERSION}-jre

LABEL project="mc-panel"

# ไม่สร้าง user ใน image — agent บังคับ User: 1000:1000 ตอน create container
# HOME ชี้เข้า /mc เพราะ modded server บางตัวชอบเขียน cache ลง $HOME
ENV HOME=/mc
WORKDIR /mc

STOPSIGNAL SIGTERM

# launch.sh ถูก generate ตอน provision — ต้อง exec java เป็น process สุดท้ายเสมอ
# เพื่อให้ java เป็น PID 1 (รับ stdin จาก docker attach + รับ SIGTERM ตรง)
CMD ["/bin/sh", "/mc/.mcpanel/launch.sh"]
