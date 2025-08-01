// Code generated: Fixed color map for known classes, generator for unknown words.

package model

import "image/color"

// ClassLabel is an enum of all YOLO class labels.
type ClassLabel string

const (
	Person        ClassLabel = "person"
	Bicycle       ClassLabel = "bicycle"
	Car           ClassLabel = "car"
	Motorcycle    ClassLabel = "motorcycle"
	Airplane      ClassLabel = "airplane"
	Bus           ClassLabel = "bus"
	Train         ClassLabel = "train"
	Truck         ClassLabel = "truck"
	Boat          ClassLabel = "boat"
	TrafficLight  ClassLabel = "traffic light"
	FireHydrant   ClassLabel = "fire hydrant"
	StopSign      ClassLabel = "stop sign"
	ParkingMeter  ClassLabel = "parking meter"
	Bench         ClassLabel = "bench"
	Bird          ClassLabel = "bird"
	Cat           ClassLabel = "cat"
	Dog           ClassLabel = "dog"
	Horse         ClassLabel = "horse"
	Sheep         ClassLabel = "sheep"
	Cow           ClassLabel = "cow"
	Elephant      ClassLabel = "elephant"
	Bear          ClassLabel = "bear"
	Zebra         ClassLabel = "zebra"
	Giraffe       ClassLabel = "giraffe"
	Backpack      ClassLabel = "backpack"
	Umbrella      ClassLabel = "umbrella"
	Handbag       ClassLabel = "handbag"
	Tie           ClassLabel = "tie"
	Suitcase      ClassLabel = "suitcase"
	Frisbee       ClassLabel = "frisbee"
	Skis          ClassLabel = "skis"
	Snowboard     ClassLabel = "snowboard"
	SportsBall    ClassLabel = "sports ball"
	Kite          ClassLabel = "kite"
	BaseballBat   ClassLabel = "baseball bat"
	BaseballGlove ClassLabel = "baseball glove"
	Skateboard    ClassLabel = "skateboard"
	Surfboard     ClassLabel = "surfboard"
	TennisRacket  ClassLabel = "tennis racket"
	Bottle        ClassLabel = "bottle"
	WineGlass     ClassLabel = "wine glass"
	Cup           ClassLabel = "cup"
	Fork          ClassLabel = "fork"
	Knife         ClassLabel = "knife"
	Spoon         ClassLabel = "spoon"
	Bowl          ClassLabel = "bowl"
	Banana        ClassLabel = "banana"
	Apple         ClassLabel = "apple"
	Sandwich      ClassLabel = "sandwich"
	Orange        ClassLabel = "orange"
	Broccoli      ClassLabel = "broccoli"
	Carrot        ClassLabel = "carrot"
	HotDog        ClassLabel = "hot dog"
	Pizza         ClassLabel = "pizza"
	Donut         ClassLabel = "donut"
	Cake          ClassLabel = "cake"
	Chair         ClassLabel = "chair"
	Couch         ClassLabel = "couch"
	PottedPlant   ClassLabel = "potted plant"
	Bed           ClassLabel = "bed"
	DiningTable   ClassLabel = "dining table"
	Toilet        ClassLabel = "toilet"
	TV            ClassLabel = "tv"
	Laptop        ClassLabel = "laptop"
	Mouse         ClassLabel = "mouse"
	Remote        ClassLabel = "remote"
	Keyboard      ClassLabel = "keyboard"
	CellPhone     ClassLabel = "cell phone"
	Microwave     ClassLabel = "microwave"
	Oven          ClassLabel = "oven"
	Toaster       ClassLabel = "toaster"
	Sink          ClassLabel = "sink"
	Refrigerator  ClassLabel = "refrigerator"
	Book          ClassLabel = "book"
	Clock         ClassLabel = "clock"
	Vase          ClassLabel = "vase"
	Scissors      ClassLabel = "scissors"
	TeddyBear     ClassLabel = "teddy bear"
	HairDrier     ClassLabel = "hair drier"
	Toothbrush    ClassLabel = "toothbrush"
)

// Fixed color map for known classes.
var classLabelColors = map[ClassLabel]color.RGBA{
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

// ClassLabelColor returns a fixed color for known classes, or generates a color for unknown words.
func ClassLabelColor(label string) color.RGBA {
	if col, ok := classLabelColors[ClassLabel(label)]; ok {
		return col
	}
	// Generate deterministic color for unknown words
	var hash uint32 = 2166136261
	for i := 0; i < len(label); i++ {
		hash = (hash * 16777619) ^ uint32(label[i])
	}
	r := uint8((hash >> 16) & 0xFF)
	g := uint8((hash >> 8) & 0xFF)
	b := uint8(hash & 0xFF)
	return color.RGBA{r, g, b, 255}
}
