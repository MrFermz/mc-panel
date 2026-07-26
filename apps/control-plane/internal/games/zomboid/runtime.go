package zomboid

// runtimeImage = image ที่ job start_server สั่งให้ agent ใช้กับ instance ของเกมนี้
//
// ต่างจาก Minecraft ที่เลือก Java image ตามเวอร์ชันของเกม: PZ พก JVM มาเองใน artifact
// ที่ Steam ติดตั้งให้ image จึงต้องการแค่ SteamCMD + lib ของ Linux ที่ตัวเกมใช้ —
// ตัวเดียวจบทุก variant/branch
//
// ค่านี้ **ต้องตรงกับ runtimeImage ของ node-agent** ซึ่งเป็นคน build image นี้บน node
// (namespace ฝั่ง agent override ได้ด้วย GM_RUNTIME_IMAGE_NAMESPACE — ตั้งแล้วต้องตั้งให้
// ตรงกันทั้งสองฝั่ง ไม่งั้น start_server จะสั่งใช้ image คนละชื่อกับที่ agent เตรียมไว้)
const runtimeImageRef = "game-manager/runtime-steam:1"

// RuntimeImage = games.VersionSpec.RuntimeImage
func RuntimeImage(_, _ string) string { return runtimeImageRef }
