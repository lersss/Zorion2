package routegame

import "zorion/internal/models"

// Passport — «паспорт перелёта»: производная (не хранилище) из уже открытых
// данных мира. Считается вызывающим (offer/блок /travel) и переиспользуется
// любой мини-игрой; в JSON-контракте — поля как у клиента ЧК3.
type Passport struct {
	Dist             float64        `json:"dist"` // расстояние от старта сегмента P до цели (его считает вызывающий)
	From             PassportStar   `json:"from"`
	To               PassportStar   `json:"to"`
	DestinationBelts []PassportBelt `json:"destination_belts"` // только Visible == true; пустой список — [] (не null)
}

// PassportStar — открытые свойства звезды/системы конца участка.
type PassportStar struct {
	SpectralClass string `json:"spectral_class"` // пусто = экзотика
	StarType      string `json:"star_type"`      // star/white_dwarf/neutron/black_hole/protostar
	SystemType    string `json:"system_type"`    // single/binary/multiple
	Temperature   int    `json:"temperature"`    // в K; в UI конвертирует клиент (°C)
}

// PassportBelt — видимый пояс системы назначения (скрытый игроку не отдаём).
type PassportBelt struct {
	Kind     string  `json:"kind"` // asteroid/kuiper/oort/debris/dust_ring
	Name     string  `json:"name"`
	RadiusAU float64 `json:"radius_au"`
	WidthAU  float64 `json:"width_au"`
}

// BuildPassport собирает паспорт перелёта: поля звёзд копируются как есть,
// пояса — только Visible == true, в исходном порядке. Масса/возраст звезды,
// регион, аномалии и пояса «по пути» в паспорт не входят (спека §5.1/§5.2).
func BuildPassport(dist float64, from, to models.World, destinationBelts []models.Belt) Passport {
	belts := make([]PassportBelt, 0, len(destinationBelts))
	for _, b := range destinationBelts {
		if !b.Visible {
			continue
		}
		belts = append(belts, PassportBelt{
			Kind:     b.Kind,
			Name:     b.Name,
			RadiusAU: b.RadiusAU,
			WidthAU:  b.WidthAU,
		})
	}
	return Passport{
		Dist:             dist,
		From:             passportStar(from),
		To:               passportStar(to),
		DestinationBelts: belts,
	}
}

func passportStar(w models.World) PassportStar {
	return PassportStar{
		SpectralClass: w.SpectralClass,
		StarType:      w.StarType,
		SystemType:    w.SystemType,
		Temperature:   w.Temperature,
	}
}
