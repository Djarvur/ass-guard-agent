package modelrouting

// Repeated string literals extracted to constants (goconst).
const stopReasonStop = "stop"
const keyModel = "model"
const providerAnthropic = "anthropic"
const providerOpenAI = "openai"
const providerGroq = "groq"
const modelMinimaxM3 = "minimax-m3"
const modelGLM46 = "glm-4.6"
const modelGLM52 = "glm-5.2"
const modelGLM53 = "GLM-5.3" // wire-exact casing (pinned capture request.body.model)
const tierLight = "light"
const tierHeavy = "heavy"
const yesSecond = "yes-second"
const tierToolLess = "tool-less"
const tierToolFull = "tool-full"
const roleUserMsg = "user"
