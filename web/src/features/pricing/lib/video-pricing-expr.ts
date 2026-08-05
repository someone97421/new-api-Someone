export type VideoPriceItem = {
  id: string
  value: string
  pricePerSecond: string
}

export type ParsedVideoPrice = {
  optionValue: string
  tierLabel: string
  pricePerSecondUSD: number
}

export type ParsedVideoPricingExpression = {
  durationPath: string
  optionPath: string
  defaultDuration: number
  maxDuration: number
  prices: ParsedVideoPrice[]
}

const STRING_LITERAL_PATTERN = '"(?:[^"\\\\]|\\\\.)*"'
const NUMBER_LITERAL_PATTERN =
  '[+-]?(?:\\d+(?:\\.\\d*)?|\\.\\d+)(?:[eE][+-]?\\d+)?'

function parseStringLiteral(value: string): string | null {
  try {
    const parsed = JSON.parse(value)
    return typeof parsed === 'string' ? parsed : null
  } catch {
    return null
  }
}

/**
 * Parse expressions emitted by {@link buildVideoPricingExpression} so the
 * public pricing UI can present resolution-specific per-second prices instead
 * of falling back to raw expression text.
 */
export function parseVideoPricingExpression(
  expression: string
): ParsedVideoPricingExpression | null {
  let body = (expression || '').split('|||', 1)[0].trim()
  const versionMatch = body.match(/^v\d+:([\s\S]*)$/)
  if (versionMatch) body = versionMatch[1].trim()
  if (!body) return null

  const conditionRegex = new RegExp(
    `param\\((${STRING_LITERAL_PATTERN})\\)\\s*==\\s*(${STRING_LITERAL_PATTERN})\\s*\\?\\s*tier\\((${STRING_LITERAL_PATTERN}),`,
    'g'
  )
  const conditions: Array<{
    optionPath: string
    optionValue: string
    tierLabel: string
  }> = []
  let conditionMatch: RegExpExecArray | null
  while ((conditionMatch = conditionRegex.exec(body)) !== null) {
    const optionPath = parseStringLiteral(conditionMatch[1])
    const optionValue = parseStringLiteral(conditionMatch[2])
    const tierLabel = parseStringLiteral(conditionMatch[3])
    if (optionPath == null || optionValue == null || tierLabel == null) {
      return null
    }
    conditions.push({ optionPath, optionValue, tierLabel })
  }
  if (conditions.length === 0) return null

  const tierRegex = new RegExp(
    `tier\\((${STRING_LITERAL_PATTERN}),\\s*min\\(max\\(param\\((${STRING_LITERAL_PATTERN})\\)\\s*==\\s*nil\\s*\\?\\s*(${NUMBER_LITERAL_PATTERN})\\s*:\\s*number\\(param\\((${STRING_LITERAL_PATTERN})\\)\\),\\s*0\\),\\s*(${NUMBER_LITERAL_PATTERN})\\)\\s*\\*\\s*\\((${NUMBER_LITERAL_PATTERN})\\s*\\/\\s*(${NUMBER_LITERAL_PATTERN})\\)\\s*\\*\\s*1000000\\)`,
    'g'
  )
  const tiers = new Map<
    string,
    {
      durationPath: string
      defaultDuration: number
      maxDuration: number
      pricePerSecondUSD: number
    }
  >()
  let tierMatch: RegExpExecArray | null
  while ((tierMatch = tierRegex.exec(body)) !== null) {
    const tierLabel = parseStringLiteral(tierMatch[1])
    const durationPath = parseStringLiteral(tierMatch[2])
    const repeatedDurationPath = parseStringLiteral(tierMatch[4])
    const defaultDuration = Number(tierMatch[3])
    const maxDuration = Number(tierMatch[5])
    const sourcePrice = Number(tierMatch[6])
    const exchangeRate = Number(tierMatch[7])
    if (
      tierLabel == null ||
      durationPath == null ||
      repeatedDurationPath !== durationPath ||
      !Number.isFinite(defaultDuration) ||
      defaultDuration <= 0 ||
      !Number.isFinite(maxDuration) ||
      maxDuration <= 0 ||
      defaultDuration > maxDuration ||
      !Number.isFinite(sourcePrice) ||
      sourcePrice < 0 ||
      !Number.isFinite(exchangeRate) ||
      exchangeRate <= 0
    ) {
      return null
    }
    tiers.set(tierLabel, {
      durationPath,
      defaultDuration,
      maxDuration,
      pricePerSecondUSD: sourcePrice / exchangeRate,
    })
  }

  const optionPath = conditions[0].optionPath
  const firstTier = tiers.get(conditions[0].tierLabel)
  if (!firstTier) return null

  const seenOptions = new Set<string>()
  const prices: ParsedVideoPrice[] = []
  for (const condition of conditions) {
    const tier = tiers.get(condition.tierLabel)
    if (
      !tier ||
      condition.optionPath !== optionPath ||
      tier.durationPath !== firstTier.durationPath ||
      tier.defaultDuration !== firstTier.defaultDuration ||
      tier.maxDuration !== firstTier.maxDuration ||
      seenOptions.has(condition.optionValue)
    ) {
      return null
    }
    seenOptions.add(condition.optionValue)
    prices.push({
      optionValue: condition.optionValue,
      tierLabel: condition.tierLabel,
      pricePerSecondUSD: tier.pricePerSecondUSD,
    })
  }

  return {
    durationPath: firstTier.durationPath,
    optionPath,
    defaultDuration: firstTier.defaultDuration,
    maxDuration: firstTier.maxDuration,
    prices,
  }
}

export function buildVideoPricingExpression(
  durationPath: string,
  optionPath: string,
  exchangeRate: string,
  defaultDuration: string,
  maxDuration: string,
  items: VideoPriceItem[],
  fallbackItemId: string
): string | null {
  const normalizedDurationPath = durationPath.trim()
  const normalizedOptionPath = optionPath.trim()
  const rate = Number(exchangeRate)
  const durationDefault = Number(defaultDuration)
  const durationLimit = Number(maxDuration)
  const validItems = items.filter(
    (item) =>
      item.value.trim() &&
      item.pricePerSecond.trim() &&
      Number.isFinite(Number(item.pricePerSecond)) &&
      Number(item.pricePerSecond) >= 0
  )
  const optionValues = new Set(validItems.map((item) => item.value.trim()))
  if (
    !normalizedDurationPath ||
    !normalizedOptionPath ||
    !Number.isFinite(rate) ||
    rate <= 0 ||
    !Number.isFinite(durationDefault) ||
    durationDefault <= 0 ||
    durationDefault > durationLimit ||
    !Number.isFinite(durationLimit) ||
    durationLimit <= 0 ||
    durationLimit > 3600 ||
    validItems.length === 0 ||
    optionValues.size !== validItems.length
  ) {
    return null
  }

  const durationParam = `param(${JSON.stringify(normalizedDurationPath)})`
  const optionParam = `param(${JSON.stringify(normalizedOptionPath)})`
  const durationExpr = `min(max(${durationParam} == nil ? ${durationDefault} : number(${durationParam}), 0), ${durationLimit})`
  const costExpr = (item: VideoPriceItem, tierName = item.value.trim()) =>
    `tier(${JSON.stringify(tierName)}, ${durationExpr} * (${Number(item.pricePerSecond)} / ${rate}) * 1000000)`
  const fallbackItem =
    validItems.find((item) => item.id === fallbackItemId) || validItems[0]
  let expression = costExpr(
    fallbackItem,
    `unknown_${fallbackItem.value.trim()}`
  )

  for (let index = validItems.length - 1; index >= 0; index--) {
    const item = validItems[index]
    expression = `${optionParam} == ${JSON.stringify(item.value.trim())} ? ${costExpr(item)} : ${expression}`
  }
  return expression
}
