# runtime-java — base image สำหรับ instance ของเกมที่รันบน JVM
# build 4 tag ด้วย make runtime-images:
#   game-manager/runtime-java:8   (Java 8)
#   game-manager/runtime-java:17  (Java 17)
#   game-manager/runtime-java:21  (Java 21)
#   game-manager/runtime-java:25  (Java 25 — fallback ของเวอร์ชันที่ใหม่กว่าที่ definition รู้จัก)
# ARG JAVA_VERSION รองรับค่าอื่นได้ถ้าเกมขยับไป Java ใหม่กว่านี้อีก
#
# image นี้ไม่รู้จักเกมใดเป็นพิเศษ — artifact และ launch script อยู่ใน bind mount /data
# (node-agent เป็นคน provision ตาม game definition)
# container ถูกสร้างโดย node-agent ด้วย: cap-drop=ALL, no-new-privileges,
# user 1000:1000, memory limit, network game-manager-servers เท่านั้น
ARG JAVA_VERSION=21
FROM eclipse-temurin:${JAVA_VERSION}-jre

LABEL project="game-manager"

# ไม่สร้าง user ใน image — agent บังคับ User: 1000:1000 ตอน create container
# HOME ชี้เข้า /data เพราะ server ที่โหลด mod บางตัวชอบเขียน cache ลง $HOME
ENV HOME=/data
WORKDIR /data

STOPSIGNAL SIGTERM

# launch.sh ถูก generate ตอน provision — ต้อง exec runtime เป็น process สุดท้ายเสมอ
# เพื่อให้เป็น PID 1 (รับ stdin จาก docker attach + รับ SIGTERM ตรง)
CMD ["/bin/sh", "/data/.gamemanager/launch.sh"]
