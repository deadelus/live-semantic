package entities

import (
	"image/color"
	"live-semantic/internal/implementation/drawer"
)

// BoxID values for the 80 COCO/YOLO11s classes, duplicating the Class_*
// string constants from class.go as drawer.BoxID so they can key
// BoxIDColors below.
const (
	Person        drawer.BoxID = "person"
	Bicycle       drawer.BoxID = "bicycle"
	Car           drawer.BoxID = "car"
	Motorcycle    drawer.BoxID = "motorcycle"
	Airplane      drawer.BoxID = "airplane"
	Bus           drawer.BoxID = "bus"
	Train         drawer.BoxID = "train"
	Truck         drawer.BoxID = "truck"
	Boat          drawer.BoxID = "boat"
	TrafficLight  drawer.BoxID = "traffic light"
	FireHydrant   drawer.BoxID = "fire hydrant"
	StopSign      drawer.BoxID = "stop sign"
	ParkingMeter  drawer.BoxID = "parking meter"
	Bench         drawer.BoxID = "bench"
	Bird          drawer.BoxID = "bird"
	Cat           drawer.BoxID = "cat"
	Dog           drawer.BoxID = "dog"
	Horse         drawer.BoxID = "horse"
	Sheep         drawer.BoxID = "sheep"
	Cow           drawer.BoxID = "cow"
	Elephant      drawer.BoxID = "elephant"
	Bear          drawer.BoxID = "bear"
	Zebra         drawer.BoxID = "zebra"
	Giraffe       drawer.BoxID = "giraffe"
	Backpack      drawer.BoxID = "backpack"
	Umbrella      drawer.BoxID = "umbrella"
	Handbag       drawer.BoxID = "handbag"
	Tie           drawer.BoxID = "tie"
	Suitcase      drawer.BoxID = "suitcase"
	Frisbee       drawer.BoxID = "frisbee"
	Skis          drawer.BoxID = "skis"
	Snowboard     drawer.BoxID = "snowboard"
	SportsBall    drawer.BoxID = "sports ball"
	Kite          drawer.BoxID = "kite"
	BaseballBat   drawer.BoxID = "baseball bat"
	BaseballGlove drawer.BoxID = "baseball glove"
	Skateboard    drawer.BoxID = "skateboard"
	Surfboard     drawer.BoxID = "surfboard"
	TennisRacket  drawer.BoxID = "tennis racket"
	Bottle        drawer.BoxID = "bottle"
	WineGlass     drawer.BoxID = "wine glass"
	Cup           drawer.BoxID = "cup"
	Fork          drawer.BoxID = "fork"
	Knife         drawer.BoxID = "knife"
	Spoon         drawer.BoxID = "spoon"
	Bowl          drawer.BoxID = "bowl"
	Banana        drawer.BoxID = "banana"
	Apple         drawer.BoxID = "apple"
	Sandwich      drawer.BoxID = "sandwich"
	Orange        drawer.BoxID = "orange"
	Broccoli      drawer.BoxID = "broccoli"
	Carrot        drawer.BoxID = "carrot"
	HotDog        drawer.BoxID = "hot dog"
	Pizza         drawer.BoxID = "pizza"
	Donut         drawer.BoxID = "donut"
	Cake          drawer.BoxID = "cake"
	Chair         drawer.BoxID = "chair"
	Couch         drawer.BoxID = "couch"
	PottedPlant   drawer.BoxID = "potted plant"
	Bed           drawer.BoxID = "bed"
	DiningTable   drawer.BoxID = "dining table"
	Toilet        drawer.BoxID = "toilet"
	TV            drawer.BoxID = "tv"
	Laptop        drawer.BoxID = "laptop"
	Mouse         drawer.BoxID = "mouse"
	Remote        drawer.BoxID = "remote"
	Keyboard      drawer.BoxID = "keyboard"
	CellPhone     drawer.BoxID = "cell phone"
	Microwave     drawer.BoxID = "microwave"
	Oven          drawer.BoxID = "oven"
	Toaster       drawer.BoxID = "toaster"
	Sink          drawer.BoxID = "sink"
	Refrigerator  drawer.BoxID = "refrigerator"
	Book          drawer.BoxID = "book"
	Clock         drawer.BoxID = "clock"
	Vase          drawer.BoxID = "vase"
	Scissors      drawer.BoxID = "scissors"
	TeddyBear     drawer.BoxID = "teddy bear"
	HairDrier     drawer.BoxID = "hair drier"
	Toothbrush    drawer.BoxID = "toothbrush"
)

// BoxIDColors is a fixed, hand-picked color per known class, used by the
// drawer to give each class a stable, distinguishable box color.
var BoxIDColors = map[drawer.BoxID]color.RGBA{
	Person:        {255, 0, 0, 255},
	Bicycle:       {0, 128, 255, 255},
	Car:           {0, 255, 0, 255},
	Motorcycle:    {255, 128, 0, 255},
	Airplane:      {128, 0, 255, 255},
	Bus:           {255, 255, 0, 255},
	Train:         {0, 255, 255, 255},
	Truck:         {255, 0, 255, 255},
	Boat:          {0, 0, 255, 255},
	TrafficLight:  {128, 255, 0, 255},
	FireHydrant:   {255, 0, 128, 255},
	StopSign:      {255, 128, 128, 255},
	ParkingMeter:  {128, 128, 255, 255},
	Bench:         {128, 128, 0, 255},
	Bird:          {0, 128, 128, 255},
	Cat:           {128, 0, 0, 255},
	Dog:           {0, 128, 0, 255},
	Horse:         {0, 0, 128, 255},
	Sheep:         {128, 255, 255, 255},
	Cow:           {255, 255, 128, 255},
	Elephant:      {255, 128, 255, 255},
	Bear:          {128, 255, 128, 255},
	Zebra:         {255, 0, 255, 255},
	Giraffe:       {0, 255, 128, 255},
	Backpack:      {128, 0, 128, 255},
	Umbrella:      {0, 128, 255, 255},
	Handbag:       {255, 128, 0, 255},
	Tie:           {128, 128, 255, 255},
	Suitcase:      {255, 255, 0, 255},
	Frisbee:       {0, 255, 255, 255},
	Skis:          {255, 0, 255, 255},
	Snowboard:     {0, 0, 255, 255},
	SportsBall:    {128, 255, 0, 255},
	Kite:          {255, 0, 128, 255},
	BaseballBat:   {255, 128, 128, 255},
	BaseballGlove: {128, 128, 255, 255},
	Skateboard:    {128, 128, 0, 255},
	Surfboard:     {0, 128, 128, 255},
	TennisRacket:  {128, 0, 0, 255},
	Bottle:        {0, 128, 0, 255},
	WineGlass:     {0, 0, 128, 255},
	Cup:           {128, 255, 255, 255},
	Fork:          {255, 255, 128, 255},
	Knife:         {255, 128, 255, 255},
	Spoon:         {128, 255, 128, 255},
	Bowl:          {255, 0, 255, 255},
	Banana:        {0, 255, 128, 255},
	Apple:         {128, 0, 128, 255},
	Sandwich:      {0, 128, 255, 255},
	Orange:        {255, 128, 0, 255},
	Broccoli:      {128, 128, 255, 255},
	Carrot:        {255, 255, 0, 255},
	HotDog:        {0, 255, 255, 255},
	Pizza:         {255, 0, 255, 255},
	Donut:         {0, 0, 255, 255},
	Cake:          {128, 255, 0, 255},
	Chair:         {255, 0, 128, 255},
	Couch:         {255, 128, 128, 255},
	PottedPlant:   {128, 128, 255, 255},
	Bed:           {255, 255, 0, 255},
	DiningTable:   {0, 255, 255, 255},
	Toilet:        {255, 0, 255, 255},
	TV:            {0, 0, 255, 255},
	Laptop:        {128, 255, 0, 255},
	Mouse:         {255, 0, 128, 255},
	Remote:        {255, 128, 128, 255},
	Keyboard:      {128, 128, 255, 255},
	CellPhone:     {255, 255, 0, 255},
	Microwave:     {0, 255, 255, 255},
	Oven:          {255, 0, 255, 255},
	Toaster:       {0, 0, 255, 255},
	Sink:          {128, 255, 0, 255},
	Refrigerator:  {255, 0, 128, 255},
	Book:          {255, 128, 128, 255},
	Clock:         {128, 128, 255, 255},
	Vase:          {255, 255, 0, 255},
	Scissors:      {0, 255, 255, 255},
	TeddyBear:     {255, 0, 255, 255},
	HairDrier:     {0, 0, 255, 255},
	Toothbrush:    {128, 255, 0, 255},
}

// BoxColor returns the fixed color for a known BoxID, or gray as a
// fallback for anything not in BoxIDColors (e.g. a future non-COCO class).
func BoxColor(ID drawer.BoxID) color.RGBA {
	if col, exists := BoxIDColors[ID]; exists {
		return col
	}
	return color.RGBA{128, 128, 128, 255}
}
